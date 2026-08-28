package initjob

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

const runColumns = `id::text,source_root,project,tags::text,recursive,status,discovered_count,photo_count,media_count,cataloged_count,photo_ai_count,media_ai_count,failed_count,current_path,error,pause_requested,cancel_requested,discovery_complete,created_at,started_at,heartbeat_at,finished_at`
const runSelect = `SELECT ` + runColumns + ` FROM init_runs`

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Run, error) {
	tags, e := json.Marshal(input.Tags)
	if e != nil {
		return Run{}, e
	}
	return scanRun(r.db.QueryRowContext(ctx, `WITH inserted AS (INSERT INTO init_runs(source_root,project,tags,recursive) VALUES($1,$2,$3::jsonb,$4) RETURNING *) SELECT `+runColumns+` FROM inserted`, input.SourceRoot, input.Project, tags, input.Recursive))
}
func (r *PostgresRepository) Get(ctx context.Context, id string) (Run, error) {
	run, e := scanRun(r.db.QueryRowContext(ctx, runSelect+` WHERE id=$1`, id))
	if errors.Is(e, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return run, e
}
func (r *PostgresRepository) List(ctx context.Context, status string, limit int) ([]Run, error) {
	rows, e := r.db.QueryContext(ctx, runSelect+` WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT $2`, status, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		run, e := scanRun(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,run_id::text,source_path,media_type,size_bytes,modified_at,file_id,status,COALESCE(asset_id::text,''),error,attempt_count FROM init_items WHERE run_id=$1 ORDER BY id LIMIT $2`, id, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		item, e := scanItem(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RequestPause(ctx context.Context, id string) error {
	return execExists(ctx, r.db, `UPDATE init_runs SET pause_requested=true WHERE id=$1 AND status NOT IN ('completed','completed_with_errors','failed','cancelled')`, id)
}
func (r *PostgresRepository) Cancel(ctx context.Context, id string) error {
	return execExists(ctx, r.db, `UPDATE init_runs SET cancel_requested=true WHERE id=$1 AND status NOT IN ('completed','completed_with_errors','failed','cancelled')`, id)
}
func (r *PostgresRepository) Resume(ctx context.Context, id string) (Run, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return Run{}, e
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM init_runs WHERE id=$1 FOR UPDATE`, id).Scan(&status); errors.Is(e, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	} else if e != nil {
		return Run{}, e
	}
	if status == "completed" {
		return Run{}, errors.New("completed init run has nothing to resume")
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=CASE WHEN status='cataloging' THEN 'discovered' ELSE 'cataloged' END,lease_owner='',lease_expires_at=NULL,updated_at=now() WHERE run_id=$1 AND status IN ('cataloging','ai_running')`, id)
	if e != nil {
		return Run{}, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=CASE WHEN asset_id IS NULL THEN 'discovered' ELSE 'cataloged' END,error='',lease_owner='',lease_expires_at=NULL,updated_at=now() WHERE run_id=$1 AND status='failed'`, id)
	if e != nil {
		return Run{}, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_runs SET status='pending',failed_count=0,pause_requested=false,cancel_requested=false,error='',heartbeat_at=NULL,finished_at=NULL WHERE id=$1`, id)
	if e != nil {
		return Run{}, e
	}
	run, e := scanRun(tx.QueryRowContext(ctx, runSelect+` WHERE id=$1`, id))
	if e != nil {
		return Run{}, e
	}
	if e = tx.Commit(); e != nil {
		return Run{}, e
	}
	return run, nil
}
func (r *PostgresRepository) ClaimRun(ctx context.Context, owner string) (Run, bool, error) {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return Run{}, false, e
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	e = tx.QueryRowContext(ctx, `SELECT id::text FROM init_runs WHERE status='pending' OR (status IN ('scanning','cataloging','photo_ai','media_ai') AND heartbeat_at<now()-interval '2 minutes') ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if e != nil {
		return Run{}, false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=CASE WHEN status='cataloging' THEN 'discovered' ELSE 'cataloged' END,lease_owner='',lease_expires_at=NULL,updated_at=now() WHERE run_id=$1 AND status IN ('cataloging','ai_running') AND (lease_expires_at IS NULL OR lease_expires_at<now())`, id)
	if e != nil {
		return Run{}, false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_runs SET status='scanning',started_at=COALESCE(started_at,now()),heartbeat_at=now(),error='' WHERE id=$1`, id)
	if e != nil {
		return Run{}, false, e
	}
	run, e := scanRun(tx.QueryRowContext(ctx, runSelect+` WHERE id=$1`, id))
	if e != nil {
		return Run{}, false, e
	}
	if e = tx.Commit(); e != nil {
		return Run{}, false, e
	}
	return run, true, nil
}
func (r *PostgresRepository) AddDiscovered(ctx context.Context, id string, files []Discovered) error {
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	for _, f := range files {
		_, e = tx.ExecContext(ctx, `INSERT INTO init_items(run_id,source_path,media_type,size_bytes,modified_at,file_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(run_id,source_path) DO UPDATE SET size_bytes=EXCLUDED.size_bytes,modified_at=EXCLUDED.modified_at,file_id=EXCLUDED.file_id,updated_at=now()`, id, f.Path, f.MediaType, f.SizeBytes, f.ModifiedAt, f.FileID)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_runs SET
		discovered_count=(SELECT count(*) FROM init_items WHERE run_id=$1),
		photo_count=(SELECT count(*) FROM init_items WHERE run_id=$1 AND media_type='photo'),
		media_count=(SELECT count(*) FROM init_items WHERE run_id=$1 AND media_type IN ('video','audio')),
		heartbeat_at=now() WHERE id=$1`, id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (r *PostgresRepository) SetRunStage(ctx context.Context, id, stage string) error {
	_, e := r.db.ExecContext(ctx, `UPDATE init_runs SET status=$2,discovery_complete=discovery_complete OR $2='cataloging',heartbeat_at=now(),current_path='' WHERE id=$1`, id, stage)
	return e
}
func (r *PostgresRepository) ClaimItems(ctx context.Context, runID, stage, mediaType string, limit int, owner string) ([]Item, error) {
	desired, running := "discovered", "cataloging"
	if stage != "catalog" {
		desired, running = "cataloged", "ai_running"
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer func() { _ = tx.Rollback() }()
	rows, e := tx.QueryContext(ctx, `SELECT id,run_id::text,source_path,media_type,size_bytes,modified_at,file_id,status,COALESCE(asset_id::text,''),error,attempt_count FROM init_items WHERE run_id=$1 AND status=$2 AND ($3='' OR media_type=$3 OR ($3='media' AND media_type IN ('video','audio'))) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $4`, runID, desired, mediaType, limit)
	if e != nil {
		return nil, e
	}
	items := []Item{}
	for rows.Next() {
		item, e := scanItem(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		items = append(items, item)
	}
	if e = rows.Close(); e != nil {
		return nil, e
	}
	for _, item := range items {
		_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=$2,attempt_count=attempt_count+1,lease_owner=$3,lease_expires_at=now()+interval '15 minutes',error='',updated_at=now() WHERE id=$1`, item.ID, running, owner)
		if e != nil {
			return nil, e
		}
		for index := range items {
			if items[index].ID == item.ID {
				items[index].AttemptCount++
			}
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return items, nil
}
func (r *PostgresRepository) CompleteCatalog(ctx context.Context, runID string, item Item, assetID string, itemErr error) error {
	status, errorText := "cataloged", ""
	if itemErr != nil {
		status, errorText = "failed", itemErr.Error()
		if item.AttemptCount < 3 {
			status = "discovered"
		}
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=$2,asset_id=NULLIF($3,'')::uuid,error=$4,lease_owner='',lease_expires_at=NULL,cataloged_at=CASE WHEN $2='cataloged' THEN now() ELSE cataloged_at END,updated_at=now() WHERE id=$1`, item.ID, status, assetID, errorText)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_runs SET cataloged_count=cataloged_count+CASE WHEN $2='cataloged' THEN 1 ELSE 0 END,failed_count=failed_count+CASE WHEN $2='failed' THEN 1 ELSE 0 END,current_path='',heartbeat_at=now() WHERE id=$1`, runID, status)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (r *PostgresRepository) CompleteAI(ctx context.Context, runID string, item Item, itemErr error) error {
	status, errorText := "completed", ""
	if itemErr != nil {
		status, errorText = "failed", itemErr.Error()
		if item.AttemptCount < 5 {
			status = "cataloged"
		}
	}
	tx, e := r.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(ctx, `UPDATE init_items SET status=$2,error=$3,lease_owner='',lease_expires_at=NULL,ai_completed_at=CASE WHEN $2='completed' THEN now() ELSE ai_completed_at END,updated_at=now() WHERE id=$1`, item.ID, status, errorText)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE init_runs SET photo_ai_count=photo_ai_count+CASE WHEN $2='completed' AND $3='photo' THEN 1 ELSE 0 END,media_ai_count=media_ai_count+CASE WHEN $2='completed' AND $3<>'photo' THEN 1 ELSE 0 END,failed_count=failed_count+CASE WHEN $2='failed' THEN 1 ELSE 0 END,current_path='',heartbeat_at=now() WHERE id=$1`, runID, status, item.MediaType)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (r *PostgresRepository) Heartbeat(ctx context.Context, id, path string) (string, error) {
	var pause, cancel bool
	e := r.db.QueryRowContext(ctx, `UPDATE init_runs SET heartbeat_at=now(),current_path=$2 WHERE id=$1 RETURNING pause_requested,cancel_requested`, id, path).Scan(&pause, &cancel)
	if e != nil {
		return "", e
	}
	if cancel {
		return "cancel", nil
	}
	if pause {
		return "pause", nil
	}
	return "continue", nil
}
func (r *PostgresRepository) Finish(ctx context.Context, id string, runErr error) error {
	if runErr != nil {
		_, e := r.db.ExecContext(ctx, `UPDATE init_runs SET status='failed',error=$2,finished_at=now(),heartbeat_at=now(),current_path='' WHERE id=$1`, id, runErr.Error())
		return e
	}
	_, e := r.db.ExecContext(ctx, `UPDATE init_runs SET status=CASE WHEN cancel_requested THEN 'cancelled' WHEN failed_count>0 THEN 'completed_with_errors' ELSE 'completed' END,finished_at=now(),heartbeat_at=now(),current_path='' WHERE id=$1`, id)
	return e
}

func execExists(ctx context.Context, db *sql.DB, query, id string) error {
	result, e := db.ExecContext(ctx, query, id)
	if e != nil {
		return e
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var exists bool
		if e = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM init_runs WHERE id=$1)`, id).Scan(&exists); e != nil {
			return e
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanRun(row rowScanner) (Run, error) {
	var run Run
	var tags string
	e := row.Scan(&run.ID, &run.SourceRoot, &run.Project, &tags, &run.Recursive, &run.Status, &run.DiscoveredCount, &run.PhotoCount, &run.MediaCount, &run.CatalogedCount, &run.PhotoAICount, &run.MediaAICount, &run.FailedCount, &run.CurrentPath, &run.Error, &run.PauseRequested, &run.CancelRequested, &run.DiscoveryComplete, &run.CreatedAt, &run.StartedAt, &run.HeartbeatAt, &run.FinishedAt)
	if e != nil {
		return Run{}, e
	}
	if e = json.Unmarshal([]byte(tags), &run.Tags); e != nil {
		return Run{}, fmt.Errorf("decode init tags: %w", e)
	}
	return run, nil
}
func scanItem(row rowScanner) (Item, error) {
	var item Item
	e := row.Scan(&item.ID, &item.RunID, &item.SourcePath, &item.MediaType, &item.SizeBytes, &item.ModifiedAt, &item.FileID, &item.Status, &item.AssetID, &item.Error, &item.AttemptCount)
	return item, e
}
