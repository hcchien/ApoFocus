package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (s *PostgresStore) Update(ctx context.Context, id string, input PhotoUpdate) (Photo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Photo{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM photos WHERE id=$1 FOR UPDATE`, id).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		return Photo{}, ErrNotFound
	} else if err != nil {
		return Photo{}, err
	}
	if input.Revision == nil || *input.Revision != revision {
		return Photo{}, ErrConflict
	}
	projectID, err := updateProject(ctx, tx, input.Project)
	if err != nil {
		return Photo{}, err
	}
	metadata, err := optionalJSON(input.UserMetadata)
	if err != nil {
		return Photo{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE photos SET
		title=CASE WHEN $2::boolean THEN NULLIF(trim($3),'') ELSE title END,
		project_id=CASE WHEN $4::boolean THEN $5::uuid ELSE project_id END,
		taken_at=COALESCE($6,taken_at),capture_year=CASE WHEN $6::timestamptz IS NULL THEN capture_year ELSE EXTRACT(YEAR FROM $6::timestamptz)::smallint END,
		camera=CASE WHEN $7::boolean THEN NULLIF(trim($8),'') ELSE camera END,
		lens=CASE WHEN $9::boolean THEN NULLIF(trim($10),'') ELSE lens END,
		aperture=CASE WHEN $11::boolean THEN NULLIF(trim($12),'') ELSE aperture END,
		shutter_speed=CASE WHEN $13::boolean THEN NULLIF(trim($14),'') ELSE shutter_speed END,
		iso=CASE WHEN $15::boolean THEN NULLIF($16,0) ELSE iso END,
		focal_length=CASE WHEN $17::boolean THEN NULLIF(trim($18),'') ELSE focal_length END,
		location_name=CASE WHEN $19::boolean THEN NULLIF(trim($20),'') WHEN $23 THEN NULL ELSE location_name END,
		latitude=CASE WHEN $19::boolean THEN $21 WHEN $23 THEN NULL ELSE latitude END,
		longitude=CASE WHEN $19::boolean THEN $22 WHEN $23 THEN NULL ELSE longitude END,
		description=CASE WHEN $24::boolean THEN $25 ELSE description END,
		copyright=CASE WHEN $26::boolean THEN $27 ELSE copyright END,
		rating=CASE WHEN $28::boolean THEN $29 ELSE rating END,
		favorite=CASE WHEN $30::boolean THEN $31 ELSE favorite END,
		user_metadata=CASE WHEN $32::boolean THEN $33::jsonb ELSE user_metadata END,
		revision=revision+1,updated_at=now() WHERE id=$1`, id,
		input.Title != nil, stringValue(input.Title), input.Project != nil, nullableUUID(projectID), input.TakenAt,
		input.Camera != nil, stringValue(input.Camera), input.Lens != nil, stringValue(input.Lens),
		input.Aperture != nil, stringValue(input.Aperture), input.ShutterSpeed != nil, stringValue(input.ShutterSpeed),
		input.ISO != nil, intValue(input.ISO), input.FocalLength != nil, stringValue(input.FocalLength),
		input.Location != nil, locationName(input.Location), latitude(input.Location), longitude(input.Location), input.ClearLocation,
		input.Description != nil, stringValue(input.Description), input.Copyright != nil, stringValue(input.Copyright),
		input.Rating != nil, intValue(input.Rating), input.Favorite != nil, boolValue(input.Favorite),
		input.UserMetadata != nil, metadata)
	if err != nil {
		return Photo{}, fmt.Errorf("update photo: %w", err)
	}
	if input.Tags != nil {
		if err := replacePhotoTags(ctx, tx, id, *input.Tags); err != nil {
			return Photo{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photos SET tags_user_edited=true WHERE id=$1`, id); err != nil {
			return Photo{}, err
		}
	}
	if err := replacePhotoRelations(ctx, tx, id, input); err != nil {
		return Photo{}, err
	}
	if err := tx.Commit(); err != nil {
		return Photo{}, err
	}
	return s.Get(ctx, id)
}

func (s *PostgresStore) UpdateMedia(ctx context.Context, mediaType, id string, input MediaUpdate) (MediaAsset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaAsset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM media_assets WHERE id=$1 AND media_type=$2 FOR UPDATE`, id, mediaType).Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		return MediaAsset{}, ErrNotFound
	} else if err != nil {
		return MediaAsset{}, err
	}
	if input.Revision == nil || *input.Revision != revision {
		return MediaAsset{}, ErrConflict
	}
	projectID, err := updateProject(ctx, tx, input.Project)
	if err != nil {
		return MediaAsset{}, err
	}
	metadata, err := optionalJSON(input.UserMetadata)
	if err != nil {
		return MediaAsset{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE media_assets SET
		title=CASE WHEN $2::boolean THEN NULLIF(trim($3),'') ELSE title END,
		project_id=CASE WHEN $4::boolean THEN $5::uuid ELSE project_id END,
		recorded_at=COALESCE($6,recorded_at),capture_year=CASE WHEN $6::timestamptz IS NULL THEN capture_year ELSE EXTRACT(YEAR FROM $6::timestamptz)::smallint END,
		description=CASE WHEN $7::boolean THEN $8 ELSE description END,
		copyright=CASE WHEN $9::boolean THEN $10 ELSE copyright END,
		rating=CASE WHEN $11::boolean THEN $12 ELSE rating END,
		favorite=CASE WHEN $13::boolean THEN $14 ELSE favorite END,
		transcript=CASE WHEN $15::boolean THEN $16 ELSE transcript END,
		user_metadata=CASE WHEN $17::boolean THEN $18::jsonb ELSE user_metadata END,
		revision=revision+1,updated_at=now() WHERE id=$1`, id,
		input.Title != nil, stringValue(input.Title), input.Project != nil, nullableUUID(projectID), input.RecordedAt,
		input.Description != nil, stringValue(input.Description), input.Copyright != nil, stringValue(input.Copyright),
		input.Rating != nil, intValue(input.Rating), input.Favorite != nil, boolValue(input.Favorite),
		input.Transcript != nil, stringValue(input.Transcript), input.UserMetadata != nil, metadata)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("update media: %w", err)
	}
	if input.Tags != nil {
		if err := replaceMediaTags(ctx, tx, id, *input.Tags); err != nil {
			return MediaAsset{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_assets SET tags_user_edited=true WHERE id=$1`, id); err != nil {
			return MediaAsset{}, err
		}
	}
	if input.Transcript != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE media_assets SET transcript_user_edited=true WHERE id=$1`, id); err != nil {
			return MediaAsset{}, err
		}
	}
	if err := replaceMediaRelations(ctx, tx, id, mediaType, input); err != nil {
		return MediaAsset{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaAsset{}, err
	}
	return s.GetMedia(ctx, mediaType, id)
}

func updateProject(ctx context.Context, tx *sql.Tx, value *string) (string, error) {
	if value == nil {
		return "", nil
	}
	name := strings.TrimSpace(*value)
	if name == "" {
		return "", nil
	}
	var id string
	err := tx.QueryRowContext(ctx, `INSERT INTO projects(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, name).Scan(&id)
	return id, err
}

func replacePhotoTags(ctx context.Context, tx *sql.Tx, photoID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM photo_tags WHERE photo_id=$1`, photoID); err != nil {
		return err
	}
	for _, tag := range cleanTags(tags) {
		var tagID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO photo_tags(photo_id,tag_id,source) VALUES($1,$2,'user')`, photoID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func replaceMediaTags(ctx context.Context, tx *sql.Tx, assetID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_asset_tags WHERE media_asset_id=$1`, assetID); err != nil {
		return err
	}
	for _, tag := range cleanTags(tags) {
		var tagID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_asset_tags(media_asset_id,tag_id,source) VALUES($1,$2,'user')`, assetID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func cleanTags(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}
func optionalJSON(value *map[string]any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	b, e := json.Marshal(*value)
	return string(b), e
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
func boolValue(value *bool) bool { return value != nil && *value }
func locationName(value *Location) string {
	if value == nil {
		return ""
	}
	return value.Name
}
func latitude(value *Location) any {
	if value == nil {
		return nil
	}
	return value.Latitude
}
func longitude(value *Location) any {
	if value == nil {
		return nil
	}
	return value.Longitude
}
func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}
