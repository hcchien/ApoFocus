package mediaingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type PostgresRepository struct {
	db            *sql.DB
	storageRootID string
}

func NewPostgresRepository(db *sql.DB, storageRootIDs ...string) *PostgresRepository {
	rootID := ""
	if len(storageRootIDs) > 0 {
		rootID = storageRootIDs[0]
	}
	return &PostgresRepository{db: db, storageRootID: rootID}
}

func (r *PostgresRepository) FindByHash(ctx context.Context, hash string) (ExistingMedia, bool, error) {
	var result ExistingMedia
	var tagsJSON string
	err := r.db.QueryRowContext(ctx, `SELECT m.id::text,m.media_type,m.path,COALESCE(m.thumbnail_path,''),COALESCE((SELECT json_agg(t.name ORDER BY t.name) FROM media_asset_tags mat JOIN tags t ON t.id=mat.tag_id WHERE mat.media_asset_id=m.id),'[]')::text FROM media_assets m WHERE m.content_sha256=$1`, hash).Scan(&result.ID, &result.MediaType, &result.Path, &result.ThumbnailPath, &tagsJSON)
	if err == sql.ErrNoRows {
		return ExistingMedia{}, false, nil
	}
	if err != nil {
		return ExistingMedia{}, false, fmt.Errorf("find media by hash: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &result.Tags); err != nil {
		return ExistingMedia{}, false, err
	}
	return result, true, nil
}

func (r *PostgresRepository) Insert(ctx context.Context, record Record) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var projectID string
	if err := tx.QueryRowContext(ctx, `INSERT INTO projects(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, record.Project).Scan(&projectID); err != nil {
		return "", fmt.Errorf("upsert project: %w", err)
	}
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return "", err
	}
	var assetID string
	err = tx.QueryRowContext(ctx, `INSERT INTO media_assets(media_type,project_id,title,capture_year,recorded_at,duration_ms,mime_type,codec,dimensions,sample_rate,channels,path,thumbnail_path,content_sha256,media_url,thumbnail_url,transcript,metadata,
		storage_root_id,relative_path,file_id,availability_status,last_verified_at,thumbnail_relative_path,thumbnail_file_id,thumbnail_status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,0),NULLIF($11,0),$12,$13,$14,$15,$16,$17,$18::jsonb,NULLIF($19,'')::uuid,NULLIF($20,''),NULLIF($21,''),'available',now(),NULLIF($22,''),NULLIF($23,''),'available') RETURNING id::text`,
		record.MediaType, projectID, record.Title, record.Year, record.RecordedAt, record.DurationMS, record.MimeType, record.Codec,
		record.Dimensions, record.SampleRate, record.Channels, record.Path, record.ThumbnailPath, record.ContentSHA256,
		record.MediaURL, record.ThumbnailURL, record.Transcript, metadata, r.storageRootID, record.RelativePath,
		record.FileID, record.ThumbnailRelativePath, record.ThumbnailFileID).Scan(&assetID)
	if err != nil {
		return "", fmt.Errorf("insert media asset: %w", err)
	}
	for _, name := range record.Tags {
		var tagID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, name).Scan(&tagID); err != nil {
			return "", fmt.Errorf("upsert tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_asset_tags(media_asset_id,tag_id,source) VALUES($1,$2,'shared') ON CONFLICT DO NOTHING`, assetID, tagID); err != nil {
			return "", fmt.Errorf("link media tag %q: %w", name, err)
		}
	}
	for _, segment := range record.Segments {
		tags, err := json.Marshal(segment.Tags)
		if err != nil {
			return "", err
		}
		segmentMetadata, err := json.Marshal(segment.Metadata)
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_segments(media_asset_id,segment_index,segment_type,start_ms,end_ms,keyframe_path,keyframe_url,transcript,tags,visual_embedding,audio_embedding,metadata,keyframe_relative_path,keyframe_file_id,keyframe_status,last_verified_at)
			VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9::jsonb,NULLIF($10,'')::vector,NULLIF($11,'')::vector,$12::jsonb,NULLIF($13,''),NULLIF($14,''),CASE WHEN $6='' THEN 'unknown' ELSE 'available' END,CASE WHEN $6='' THEN NULL ELSE now() END)`,
			assetID, segment.Index, segment.SegmentType, segment.StartMS, segment.EndMS, segment.KeyframePath, segment.KeyframeURL,
			segment.Transcript, tags, vectorLiteral(segment.VisualVector), vectorLiteral(segment.AudioVector), segmentMetadata,
			segment.KeyframeRelativePath, segment.KeyframeFileID); err != nil {
			return "", fmt.Errorf("insert media segment %s/%d: %w", segment.SegmentType, segment.Index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return assetID, nil
}

func vectorLiteral(vector []float32) string {
	if len(vector) == 0 {
		return ""
	}
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = fmt.Sprintf("%.8f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
