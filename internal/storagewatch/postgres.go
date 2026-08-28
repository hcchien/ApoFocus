package storagewatch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hcchien/apofocus/internal/fileidentity"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) ListRoots(ctx context.Context) ([]Root, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id::text,name,base_path,volume_id,status,last_seen_at,last_event_at FROM storage_roots ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roots := []Root{}
	for rows.Next() {
		var root Root
		if err := rows.Scan(&root.ID, &root.Name, &root.BasePath, &root.VolumeID, &root.Status, &root.LastSeenAt, &root.LastEvent); err != nil {
			return nil, err
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

func (r *PostgresRepository) EnsureRoot(ctx context.Context, configuredPath string) (Root, error) {
	absolute, err := filepath.Abs(configuredPath)
	if err != nil {
		return Root{}, err
	}
	absolute = filepath.Clean(absolute)
	configuredAbsolute := absolute
	info, statErr := os.Stat(absolute)
	if statErr == nil && !info.IsDir() {
		return Root{}, fmt.Errorf("storage root is not a directory: %s", absolute)
	}
	status, volumeID := "online", ""
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return Root{}, statErr
		}
		status = "offline"
	} else {
		if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
			absolute = resolved
		}
		identity, identityErr := fileidentity.FromPath(absolute)
		if identityErr != nil {
			return Root{}, identityErr
		}
		volumeID = identity.VolumeID
	}
	name := filepath.Base(absolute)
	var root Root
	err = r.db.QueryRowContext(ctx, `INSERT INTO storage_roots(name,base_path,volume_id,status,last_seen_at,updated_at)
		VALUES($1,$2,$3,$4,now(),now())
		ON CONFLICT(base_path) DO UPDATE SET name=EXCLUDED.name,volume_id=CASE WHEN EXCLUDED.volume_id='' THEN storage_roots.volume_id ELSE EXCLUDED.volume_id END,
		status=EXCLUDED.status,last_seen_at=CASE WHEN EXCLUDED.status='online' THEN now() ELSE storage_roots.last_seen_at END,updated_at=now()
		RETURNING id::text,name,base_path,volume_id,status,last_seen_at,last_event_at`, name, absolute, volumeID, status).
		Scan(&root.ID, &root.Name, &root.BasePath, &root.VolumeID, &root.Status, &root.LastSeenAt, &root.LastEvent)
	if err != nil {
		return Root{}, fmt.Errorf("register storage root: %w", err)
	}
	if root.Status == "offline" {
		if err := r.markRootAvailability(ctx, root.ID, "volume_offline"); err != nil {
			return Root{}, err
		}
		return root, ErrRootOffline
	}
	if err := r.backfill(ctx, root, root.BasePath, configuredAbsolute); err != nil {
		return Root{}, err
	}
	return root, nil
}

func (r *PostgresRepository) Backfill(ctx context.Context, root Root) error {
	return r.backfill(ctx, root, root.BasePath)
}

func (r *PostgresRepository) backfill(ctx context.Context, root Root, basePaths ...string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	queries := []string{
		`UPDATE photos SET storage_root_id=$1,
			relative_path=CASE WHEN path=$2 THEN '' ELSE substr(path,length($2)+2) END,
			thumbnail_relative_path=CASE WHEN thumbnail_path IS NULL THEN NULL WHEN thumbnail_path=$2 THEN '' WHEN left(thumbnail_path,length($2)+1)=$2||'/' THEN substr(thumbnail_path,length($2)+2) ELSE thumbnail_relative_path END
		 WHERE path=$2 OR left(path,length($2)+1)=$2||'/'`,
		`UPDATE media_assets SET storage_root_id=$1,
			relative_path=CASE WHEN path=$2 THEN '' ELSE substr(path,length($2)+2) END,
			thumbnail_relative_path=CASE WHEN thumbnail_path IS NULL THEN NULL WHEN thumbnail_path=$2 THEN '' WHEN left(thumbnail_path,length($2)+1)=$2||'/' THEN substr(thumbnail_path,length($2)+2) ELSE thumbnail_relative_path END
		 WHERE path=$2 OR left(path,length($2)+1)=$2||'/'`,
		`UPDATE media_segments ms SET keyframe_relative_path=CASE WHEN ms.keyframe_path=$2 THEN '' ELSE substr(ms.keyframe_path,length($2)+2) END
		 FROM media_assets ma WHERE ma.id=ms.media_asset_id AND ma.storage_root_id=$1 AND ms.keyframe_path IS NOT NULL
		 AND (ms.keyframe_path=$2 OR left(ms.keyframe_path,length($2)+1)=$2||'/')`,
	}
	seen := map[string]bool{}
	for _, basePath := range basePaths {
		basePath = filepath.Clean(basePath)
		if seen[basePath] {
			continue
		}
		seen[basePath] = true
		for _, query := range queries {
			if _, err := tx.ExecContext(ctx, query, root.ID, basePath); err != nil {
				return fmt.Errorf("backfill storage paths: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return r.VerifyKnownPaths(ctx, root)
}

func (r *PostgresRepository) VerifyKnownPaths(ctx context.Context, root Root) error {
	if _, err := os.Stat(root.BasePath); errors.Is(err, os.ErrNotExist) {
		if markErr := r.MarkRootOffline(ctx, root); markErr != nil {
			return markErr
		}
		return ErrRootOffline
	} else if err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT kind,path FROM (
			SELECT 'photo_original' AS kind,path FROM photos WHERE storage_root_id=$1
			UNION ALL SELECT 'photo_thumbnail',thumbnail_path FROM photos WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL
			UNION ALL SELECT 'media_original',path FROM media_assets WHERE storage_root_id=$1
			UNION ALL SELECT 'media_thumbnail',thumbnail_path FROM media_assets WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL
			UNION ALL SELECT 'segment_keyframe',ms.keyframe_path FROM media_segments ms JOIN media_assets ma ON ma.id=ms.media_asset_id WHERE ma.storage_root_id=$1 AND ms.keyframe_path IS NOT NULL
		) known ORDER BY path`, root.ID)
	if err != nil {
		return fmt.Errorf("list known storage paths: %w", err)
	}
	defer rows.Close()
	type knownPath struct{ kind, path string }
	known := []knownPath{}
	for rows.Next() {
		var item knownPath
		if err := rows.Scan(&item.kind, &item.path); err != nil {
			return err
		}
		known = append(known, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range known {
		canonicalPath, err := filepath.EvalSymlinks(item.path)
		if errors.Is(err, os.ErrNotExist) {
			if err := r.MarkMissing(ctx, root, item.path); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		identity, err := fileidentity.FromPath(canonicalPath)
		if err != nil {
			return err
		}
		if err := r.observePath(ctx, root, item.path, canonicalPath, identity); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) ObservePath(ctx context.Context, root Root, path string, identity fileidentity.Identity) error {
	return r.observePath(ctx, root, path, path, identity)
}

func (r *PostgresRepository) observePath(ctx context.Context, root Root, previousPath, path string, identity fileidentity.Identity) error {
	relative, err := relativePath(root.BasePath, path)
	if err != nil {
		return err
	}
	publicURL := mediaURL(relative)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	queries := []string{
		`UPDATE photos SET path=$3,relative_path=$4,image_url='/api/v1/photos/'||id::text||'/file',file_id=NULLIF($6,''),availability_status='available',last_verified_at=now(),updated_at=now()
		 WHERE storage_root_id=$1 AND ((file_id IS NOT NULL AND file_id=$6) OR path=$2)`,
		`UPDATE photos SET thumbnail_path=$3,thumbnail_relative_path=$4,thumbnail_url=$5,thumbnail_file_id=NULLIF($6,''),thumbnail_status='available',last_verified_at=now(),updated_at=now()
		 WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND ((thumbnail_file_id IS NOT NULL AND thumbnail_file_id=$6) OR thumbnail_path=$2)`,
		`UPDATE media_assets SET path=$3,relative_path=$4,media_url='/api/v1/'||CASE WHEN media_type='video' THEN 'videos' ELSE 'audios' END||'/'||id::text||'/file',file_id=NULLIF($6,''),availability_status='available',last_verified_at=now(),updated_at=now()
		 WHERE storage_root_id=$1 AND ((file_id IS NOT NULL AND file_id=$6) OR path=$2)`,
		`UPDATE media_assets SET thumbnail_path=$3,thumbnail_relative_path=$4,thumbnail_url=$5,thumbnail_file_id=NULLIF($6,''),thumbnail_status='available',last_verified_at=now(),updated_at=now()
		 WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND ((thumbnail_file_id IS NOT NULL AND thumbnail_file_id=$6) OR thumbnail_path=$2)`,
		`UPDATE media_segments ms SET keyframe_path=$3,keyframe_relative_path=$4,keyframe_url=$5,keyframe_file_id=NULLIF($6,''),keyframe_status='available',last_verified_at=now()
		 FROM media_assets ma WHERE ma.id=ms.media_asset_id AND ma.storage_root_id=$1
		 AND ((ms.keyframe_file_id IS NOT NULL AND ms.keyframe_file_id=$6) OR ms.keyframe_path=$2)`,
	}
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query, root.ID, previousPath, path, relative, publicURL, identity.FileID); err != nil {
			return fmt.Errorf("observe storage path: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE storage_roots SET status='online',last_seen_at=now(),last_event_at=now(),updated_at=now() WHERE id=$1`, root.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) MarkMissing(ctx context.Context, root Root, path string) error {
	queries := []string{
		`UPDATE photos SET availability_status='missing',last_verified_at=now(),updated_at=now() WHERE storage_root_id=$1 AND (path=$2 OR left(path,length($2)+1)=$2||'/')`,
		`UPDATE photos SET thumbnail_status='missing',last_verified_at=now(),updated_at=now() WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL AND (thumbnail_path=$2 OR left(thumbnail_path,length($2)+1)=$2||'/')`,
		`UPDATE media_assets SET availability_status='missing',last_verified_at=now(),updated_at=now() WHERE storage_root_id=$1 AND (path=$2 OR left(path,length($2)+1)=$2||'/')`,
		`UPDATE media_assets SET thumbnail_status='missing',last_verified_at=now(),updated_at=now() WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL AND (thumbnail_path=$2 OR left(thumbnail_path,length($2)+1)=$2||'/')`,
		`UPDATE media_segments ms SET keyframe_status='missing',last_verified_at=now() FROM media_assets ma WHERE ma.id=ms.media_asset_id AND ma.storage_root_id=$1 AND ms.keyframe_path IS NOT NULL AND (ms.keyframe_path=$2 OR left(ms.keyframe_path,length($2)+1)=$2||'/')`,
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range queries {
		if _, err := tx.ExecContext(ctx, query, root.ID, filepath.Clean(path)); err != nil {
			return fmt.Errorf("mark missing storage path: %w", err)
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) TouchRoot(ctx context.Context, root Root) error {
	_, err := r.db.ExecContext(ctx, `UPDATE storage_roots SET status='online',last_seen_at=now(),last_event_at=now(),updated_at=now() WHERE id=$1`, root.ID)
	return err
}

func (r *PostgresRepository) MarkRootOffline(ctx context.Context, root Root) error {
	return r.markRootAvailability(ctx, root.ID, "volume_offline")
}

func (r *PostgresRepository) markRootAvailability(ctx context.Context, rootID, status string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if status == "volume_offline" {
		if _, err := tx.ExecContext(ctx, `UPDATE storage_roots SET status='offline',last_event_at=now(),updated_at=now() WHERE id=$1`, rootID); err != nil {
			return err
		}
	}
	for _, query := range []string{
		`UPDATE photos SET availability_status=$2,last_verified_at=now() WHERE storage_root_id=$1`,
		`UPDATE photos SET thumbnail_status=$2,last_verified_at=now() WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL`,
		`UPDATE media_assets SET availability_status=$2,last_verified_at=now() WHERE storage_root_id=$1`,
		`UPDATE media_assets SET thumbnail_status=$2,last_verified_at=now() WHERE COALESCE(thumbnail_storage_root_id,storage_root_id)=$1 AND thumbnail_path IS NOT NULL`,
		`UPDATE media_segments ms SET keyframe_status=$2,last_verified_at=now() FROM media_assets ma WHERE ma.id=ms.media_asset_id AND ma.storage_root_id=$1 AND ms.keyframe_path IS NOT NULL`,
	} {
		if _, err := tx.ExecContext(ctx, query, rootID, status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func relativePath(root, path string) (string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside storage root %q", path, root)
	}
	return filepath.ToSlash(relative), nil
}

func mediaURL(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return "/media/" + strings.Join(parts, "/")
}
