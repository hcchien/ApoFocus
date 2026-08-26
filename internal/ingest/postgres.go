package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindByHash(ctx context.Context, hash string) (ExistingPhoto, bool, error) {
	var result ExistingPhoto
	var tagsJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT p.id::text, p.path, COALESCE(p.thumbnail_path, ''),
		       COALESCE((SELECT json_agg(t.name ORDER BY t.name) FROM photo_tags pt JOIN tags t ON t.id=pt.tag_id WHERE pt.photo_id=p.id), '[]')::text
		FROM photos p WHERE p.content_sha256=$1`, hash).Scan(&result.ID, &result.Path, &result.ThumbnailPath, &tagsJSON)
	if err == sql.ErrNoRows {
		return ExistingPhoto{}, false, nil
	}
	if err != nil {
		return ExistingPhoto{}, false, fmt.Errorf("find photo by hash: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &result.Tags); err != nil {
		return ExistingPhoto{}, false, err
	}
	return result, true, nil
}

func (r *PostgresRepository) Insert(ctx context.Context, record PhotoRecord) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var projectID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO projects(name) VALUES($1)
		ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name
		RETURNING id::text`, record.Project).Scan(&projectID); err != nil {
		return "", fmt.Errorf("upsert project: %w", err)
	}
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return "", err
	}
	var latitude, longitude any
	var locationName string
	if record.Location != nil {
		latitude, longitude, locationName = record.Location.Latitude, record.Location.Longitude, record.Location.Name
	}
	var photoID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO photos(
			project_id, title, capture_year, taken_at, camera, lens, aperture, shutter_speed,
			iso, focal_length, dimensions, file_type, file_size, location_name, latitude, longitude,
			path, thumbnail_path, content_sha256, image_url, thumbnail_url, aspect_ratio,
			dominant_color, metadata, embedding
		) VALUES(
			$1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''),
			NULLIF($9,0), NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), $13, NULLIF($14,''), $15, $16,
			$17, $18, $19, $20, $21, $22, NULLIF($23,''), $24::jsonb, $25::vector
		) RETURNING id::text`,
		projectID, record.Title, record.Year, record.TakenAt, record.Camera, record.Lens, record.Aperture, record.ShutterSpeed,
		record.ISO, record.FocalLength, record.Dimensions, record.FileType, humanBytes(record.FileSizeBytes), locationName,
		latitude, longitude, record.Path, record.ThumbnailPath, record.ContentSHA256, record.ImageURL, record.ThumbnailURL,
		record.AspectRatio, record.DominantColor, metadata, vectorLiteral(record.embedding)).Scan(&photoID)
	if err != nil {
		return "", fmt.Errorf("insert photo: %w", err)
	}
	for _, name := range record.Tags {
		var tagID string
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO tags(name) VALUES($1)
			ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name
			RETURNING id::text`, name).Scan(&tagID); err != nil {
			return "", fmt.Errorf("upsert tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO photo_tags(photo_id, tag_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, photoID, tagID); err != nil {
			return "", fmt.Errorf("link tag %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return photoID, nil
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = fmt.Sprintf("%.8f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func humanBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := int64(unit), 0
	for quotient := value / unit; quotient >= unit; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}
