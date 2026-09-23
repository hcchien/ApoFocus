package deepanalysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

const jobColumns = `id::text,model,prompt_version,status,selected_count,processed_count,succeeded_count,failed_count,error,cancel_requested,created_at,started_at,heartbeat_at,finished_at`

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput, model, promptVersion string) (Job, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var jobID string
	if err = tx.QueryRowContext(ctx, `INSERT INTO deep_analysis_jobs(model,prompt_version) VALUES($1,$2) RETURNING id::text`, model, promptVersion).Scan(&jobID); err != nil {
		return Job{}, err
	}
	selected := 0
	for _, photoID := range input.PhotoIDs {
		var revision int64
		var status, currentModel, currentPrompt string
		err = tx.QueryRowContext(ctx, `SELECT revision,deep_analysis_status,deep_analysis_model,deep_analysis_prompt_version FROM photos WHERE id=$1 FOR UPDATE`, photoID).Scan(&revision, &status, &currentModel, &currentPrompt)
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, ErrNotFound
		}
		if err != nil {
			return Job{}, err
		}
		if !input.Force && status == "completed" && currentModel == model && currentPrompt == promptVersion {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO deep_analysis_items(job_id,photo_id,source_revision) VALUES($1,$2,$3)`, jobID, photoID, revision); err != nil {
			return Job{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='pending' WHERE id=$1`, photoID); err != nil {
			return Job{}, err
		}
		selected++
	}
	status := "pending"
	if selected == 0 {
		status = "completed"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_jobs SET selected_count=$2,status=$3,finished_at=CASE WHEN $3='completed' THEN now() ELSE NULL END WHERE id=$1`, jobID, selected, status); err != nil {
		return Job{}, err
	}
	job, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deep_analysis_jobs WHERE id=$1`, jobID))
	if err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Job, error) {
	job, err := scanJob(r.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deep_analysis_jobs WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

func (r *PostgresRepository) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,job_id::text,photo_id::text,source_revision,status,result::text,error,attempt_count,started_at,finished_at FROM deep_analysis_items WHERE job_id=$1 ORDER BY id LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) LatestForPhoto(ctx context.Context, photoID string) (PhotoAnalysis, error) {
	var resultText string
	var item PhotoAnalysis
	err := r.db.QueryRowContext(ctx, `SELECT id::text,deep_analysis_status,deep_analysis_model,deep_analysis_prompt_version,deep_analysis::text,deep_analyzed_at FROM photos WHERE id=$1`, photoID).Scan(&item.PhotoID, &item.Status, &item.Model, &item.PromptVersion, &resultText, &item.AnalyzedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PhotoAnalysis{}, ErrNotFound
	}
	item.Result = json.RawMessage(resultText)
	return item, err
}

func (r *PostgresRepository) Cancel(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE deep_analysis_jobs SET cancel_requested=true WHERE id=$1 AND status IN ('pending','running')`, id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deep_analysis_jobs WHERE id=$1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (r *PostgresRepository) RecoverStalled(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE deep_analysis_items SET status='pending',updated_at=now() WHERE status='running';
		UPDATE photos SET deep_analysis_status='pending' WHERE deep_analysis_status='running';
		UPDATE deep_analysis_jobs SET status='pending',heartbeat_at=now(),error='' WHERE status='running';
	`)
	return err
}

func (r *PostgresRepository) ClaimNext(ctx context.Context) (Job, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM deep_analysis_jobs WHERE status='pending' OR (status='running' AND heartbeat_at<now()-interval '2 minutes') ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_items SET status='pending',updated_at=now() WHERE job_id=$1 AND status='running'`, id); err != nil {
		return Job{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='pending' WHERE id IN (SELECT photo_id FROM deep_analysis_items WHERE job_id=$1)`, id); err != nil {
		return Job{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_jobs SET status='running',started_at=COALESCE(started_at,now()),heartbeat_at=now(),error='' WHERE id=$1`, id); err != nil {
		return Job{}, false, err
	}
	job, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM deep_analysis_jobs WHERE id=$1`, id))
	if err != nil {
		return Job{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (r *PostgresRepository) NextItem(ctx context.Context, jobID string) (Item, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `SELECT i.id,i.job_id::text,i.photo_id::text,i.source_revision,i.status,i.result::text,i.error,i.attempt_count,i.started_at,i.finished_at,COALESCE(NULLIF(p.thumbnail_path,''),p.path) FROM deep_analysis_items i JOIN photos p ON p.id=i.photo_id WHERE i.job_id=$1 AND i.status='pending' ORDER BY i.id FOR UPDATE SKIP LOCKED LIMIT 1`, jobID)
	item, err := scanItemWithPath(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_items SET status='running',attempt_count=attempt_count+1,started_at=COALESCE(started_at,now()),updated_at=now() WHERE id=$1`, item.ID); err != nil {
		return Item{}, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='running' WHERE id=$1`, item.PhotoID); err != nil {
		return Item{}, false, err
	}
	item.AttemptCount++
	if err = tx.Commit(); err != nil {
		return Item{}, false, err
	}
	return item, true, nil
}

func (r *PostgresRepository) CompleteItem(ctx context.Context, job Job, item Item, result json.RawMessage, itemErr error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	status, errorText := "completed", ""
	if itemErr != nil {
		status, errorText = "failed", itemErr.Error()
		if item.AttemptCount < 3 {
			status = "pending"
		}
	}
	if len(result) == 0 || !json.Valid(result) {
		result = json.RawMessage(`{}`)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_items SET status=$2,result=$3::jsonb,error=$4,finished_at=CASE WHEN $2 IN ('completed','failed') THEN now() ELSE NULL END,updated_at=now() WHERE id=$1`, item.ID, status, result, errorText); err != nil {
		return err
	}
	if status == "completed" {
		if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='completed',deep_analysis=$2::jsonb,deep_analysis_model=$3,deep_analysis_prompt_version=$4,deep_analyzed_at=now() WHERE id=$1`, item.PhotoID, result, job.Model, job.PromptVersion); err != nil {
			return err
		}
	} else if status == "failed" {
		if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='failed' WHERE id=$1`, item.PhotoID); err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE photos SET deep_analysis_status='pending' WHERE id=$1`, item.PhotoID); err != nil {
			return err
		}
	}
	if status == "completed" || status == "failed" {
		if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_jobs SET processed_count=processed_count+1,succeeded_count=succeeded_count+CASE WHEN $2='completed' THEN 1 ELSE 0 END,failed_count=failed_count+CASE WHEN $2='failed' THEN 1 ELSE 0 END,heartbeat_at=now() WHERE id=$1`, job.ID, status); err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE deep_analysis_jobs SET heartbeat_at=now() WHERE id=$1`, job.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) Heartbeat(ctx context.Context, id string) (bool, error) {
	var cancel bool
	err := r.db.QueryRowContext(ctx, `UPDATE deep_analysis_jobs SET heartbeat_at=now() WHERE id=$1 RETURNING cancel_requested`, id).Scan(&cancel)
	return cancel, err
}

func (r *PostgresRepository) Finish(ctx context.Context, id string, runErr error) error {
	if runErr != nil {
		_, err := r.db.ExecContext(ctx, `UPDATE deep_analysis_jobs SET status='failed',error=$2,finished_at=now(),heartbeat_at=now() WHERE id=$1`, id, runErr.Error())
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE deep_analysis_jobs SET status=CASE WHEN cancel_requested THEN 'cancelled' WHEN failed_count>0 THEN 'completed_with_errors' ELSE 'completed' END,finished_at=now(),heartbeat_at=now() WHERE id=$1`, id)
	return err
}

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.Model, &job.PromptVersion, &job.Status, &job.SelectedCount, &job.ProcessedCount, &job.SucceededCount, &job.FailedCount, &job.Error, &job.CancelRequested, &job.CreatedAt, &job.StartedAt, &job.HeartbeatAt, &job.FinishedAt)
	return job, err
}

func scanItem(row rowScanner) (Item, error) {
	var item Item
	var resultText string
	err := row.Scan(&item.ID, &item.JobID, &item.PhotoID, &item.SourceRevision, &item.Status, &resultText, &item.Error, &item.AttemptCount, &item.StartedAt, &item.FinishedAt)
	if err != nil {
		return Item{}, err
	}
	item.Result = json.RawMessage(resultText)
	return item, nil
}

func scanItemWithPath(row rowScanner) (Item, error) {
	var item Item
	var resultText string
	err := row.Scan(&item.ID, &item.JobID, &item.PhotoID, &item.SourceRevision, &item.Status, &resultText, &item.Error, &item.AttemptCount, &item.StartedAt, &item.FinishedAt, &item.Path)
	if err != nil {
		return Item{}, err
	}
	if !json.Valid([]byte(resultText)) {
		return Item{}, fmt.Errorf("invalid deep analysis result for item %d", item.ID)
	}
	item.Result = json.RawMessage(resultText)
	return item, nil
}
