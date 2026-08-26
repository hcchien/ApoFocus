package batch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("batch job not found")

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

const jobColumns = `id::text,source_root,project,tags::text,recursive,auto_tags,media_types::text,status,
discovered_count,processed_count,succeeded_count,failed_count,current_path,error,cancel_requested,
created_at,started_at,finished_at`
const jobSelect = `SELECT ` + jobColumns + ` FROM batch_jobs`

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Job, error) {
	tags, err := json.Marshal(input.Tags)
	if err != nil {
		return Job{}, err
	}
	mediaTypes, err := json.Marshal(input.MediaTypes)
	if err != nil {
		return Job{}, err
	}
	return scanJob(r.db.QueryRowContext(ctx, `WITH inserted AS (
		INSERT INTO batch_jobs(source_root,project,tags,recursive,auto_tags,media_types)
		VALUES($1,$2,$3::jsonb,$4,$5,$6::jsonb) RETURNING *) SELECT `+jobColumns+` FROM inserted`, input.SourceRoot, input.Project, tags, input.Recursive, input.AutoTags, mediaTypes))
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Job, error) {
	job, err := scanJob(r.db.QueryRowContext(ctx, jobSelect+` WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (r *PostgresRepository) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,source_path,status,media_type,COALESCE(photo_id::text,''),COALESCE(media_asset_id::text,''),error,started_at,finished_at FROM batch_items WHERE job_id=$1 ORDER BY id LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Item{}
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.SourcePath, &item.Status, &item.MediaType, &item.PhotoID, &item.MediaAssetID, &item.Error, &item.StartedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) Cancel(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE batch_jobs SET cancel_requested=true WHERE id=$1 AND status NOT IN ('completed','completed_with_errors','failed','cancelled')`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM batch_jobs WHERE id=$1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *PostgresRepository) ClaimNext(ctx context.Context) (Job, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM batch_jobs
		WHERE status='pending' OR (status IN ('scanning','running') AND heartbeat_at < now()-interval '2 minutes')
		ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_jobs SET status='scanning',started_at=COALESCE(started_at,now()),heartbeat_at=now(),error='' WHERE id=$1`, id); err != nil {
		return Job{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_items SET status='pending',started_at=NULL WHERE job_id=$1 AND status='running'`, id); err != nil {
		return Job{}, false, err
	}
	job, err := scanJob(tx.QueryRowContext(ctx, jobSelect+` WHERE id=$1`, id))
	if err != nil {
		return Job{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (r *PostgresRepository) AddDiscovered(ctx context.Context, id string, files []DiscoveredFile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, file := range files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO batch_items(job_id,source_path,media_type) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, file.Path, file.MediaType); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_jobs SET discovered_count=(SELECT count(*) FROM batch_items WHERE job_id=$1),heartbeat_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) StartRunning(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE batch_jobs SET status='running',heartbeat_at=now() WHERE id=$1`, id)
	return err
}

func (r *PostgresRepository) NextItem(ctx context.Context, jobID string) (Item, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var item Item
	err = tx.QueryRowContext(ctx, `SELECT id,source_path,status,media_type,COALESCE(photo_id::text,''),COALESCE(media_asset_id::text,''),error,started_at,finished_at FROM batch_items WHERE job_id=$1 AND status='pending' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`, jobID).Scan(&item.ID, &item.SourcePath, &item.Status, &item.MediaType, &item.PhotoID, &item.MediaAssetID, &item.Error, &item.StartedAt, &item.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_items SET status='running',started_at=now(),error='' WHERE id=$1`, item.ID); err != nil {
		return Item{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Item{}, false, err
	}
	item.Status = "running"
	return item, true, nil
}

func (r *PostgresRepository) CompleteItem(ctx context.Context, jobID string, itemID int64, mediaType, assetID string, itemErr error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	status, errorText := "succeeded", ""
	var photo, media any
	if mediaType == "photo" {
		photo = assetID
	} else {
		media = assetID
	}
	if itemErr != nil {
		status, errorText, photo, media = "failed", itemErr.Error(), nil, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_items SET status=$2,photo_id=$3,media_asset_id=$4,error=$5,finished_at=now() WHERE id=$1`, itemID, status, photo, media, errorText); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE batch_jobs SET processed_count=processed_count+1,
		succeeded_count=succeeded_count+CASE WHEN $2='succeeded' THEN 1 ELSE 0 END,
		failed_count=failed_count+CASE WHEN $2='failed' THEN 1 ELSE 0 END,
		current_path='',heartbeat_at=now() WHERE id=$1`, jobID, status); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) Finish(ctx context.Context, id string, jobErr error) error {
	if jobErr != nil {
		_, err := r.db.ExecContext(ctx, `UPDATE batch_jobs SET status='failed',error=$2,finished_at=now(),heartbeat_at=now(),current_path='' WHERE id=$1`, id, jobErr.Error())
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE batch_jobs SET status=CASE WHEN cancel_requested THEN 'cancelled' WHEN failed_count>0 THEN 'completed_with_errors' ELSE 'completed' END,finished_at=now(),heartbeat_at=now(),current_path='' WHERE id=$1`, id)
	return err
}

func (r *PostgresRepository) Heartbeat(ctx context.Context, id, currentPath string) (bool, error) {
	var cancel bool
	err := r.db.QueryRowContext(ctx, `UPDATE batch_jobs SET heartbeat_at=now(),current_path=$2 WHERE id=$1 RETURNING cancel_requested`, id, currentPath).Scan(&cancel)
	return cancel, err
}

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (Job, error) {
	var job Job
	var tagsJSON, mediaTypesJSON string
	err := row.Scan(&job.ID, &job.SourceRoot, &job.Project, &tagsJSON, &job.Recursive, &job.AutoTags, &mediaTypesJSON, &job.Status, &job.DiscoveredCount, &job.ProcessedCount, &job.SucceededCount, &job.FailedCount, &job.CurrentPath, &job.Error, &job.CancelRequested, &job.CreatedAt, &job.StartedAt, &job.FinishedAt)
	if err != nil {
		return Job{}, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &job.Tags); err != nil {
		return Job{}, fmt.Errorf("decode batch tags: %w", err)
	}
	if err := json.Unmarshal([]byte(mediaTypesJSON), &job.MediaTypes); err != nil {
		return Job{}, fmt.Errorf("decode batch media types: %w", err)
	}
	return job, nil
}
