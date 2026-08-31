package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore { return &PostgresStore{db: db} }

const photoSelect = `
SELECT p.id::text, p.title, p.capture_year,
       COALESCE((SELECT COALESCE(NULLIF(rp.description,''),rp.name,'') FROM project_photos rpp JOIN projects rp ON rp.id=rpp.project_id WHERE rpp.photo_id=p.id ORDER BY COALESCE(NULLIF(rp.description,''),rp.name,''),rp.id LIMIT 1),pr.name,''), p.taken_at,
       COALESCE((SELECT json_agg(t.name ORDER BY t.name) FROM photo_tags pt JOIN tags t ON t.id = pt.tag_id WHERE pt.photo_id = p.id), '[]')::text,
       COALESCE(p.camera, ''), COALESCE(p.lens, ''), COALESCE(p.aperture, ''), COALESCE(p.shutter_speed, ''),
       COALESCE(p.iso, 0), COALESCE(p.focal_length, ''), COALESCE(p.dimensions, ''), COALESCE(p.file_type, ''),
       COALESCE(p.file_size, ''), COALESCE(p.location_name, ''), p.latitude, p.longitude,
       COALESCE(p.path, ''), COALESCE(p.thumbnail_path, ''), p.image_url, p.thumbnail_url,
       COALESCE(p.aspect_ratio, 'landscape'), COALESCE(p.dominant_color, '#757575'),
       COALESCE(p.metadata, '{}'::jsonb)::text,
       COALESCE(p.availability_status, 'unknown'), COALESCE(p.thumbnail_status, 'unknown'),
       COALESCE(p.description,''),COALESCE(p.copyright,''),COALESCE(p.rating,0),COALESCE(p.favorite,false),
       COALESCE(p.user_metadata,'{}'::jsonb)::text,COALESCE(p.revision,1),
       COALESCE(p.content_hash_status,'completed'),COALESCE(p.ai_status,'completed')
FROM photos p
LEFT JOIN projects pr ON pr.id = p.project_id`

func (s *PostgresStore) List(ctx context.Context, filter Filter) (PhotoPage, error) {
	where, args := buildWhere(filter)
	var total int
	countQuery := `SELECT count(*) FROM photos p LEFT JOIN projects pr ON pr.id = p.project_id ` + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return PhotoPage{}, fmt.Errorf("count photos: %w", err)
	}
	args = append(args, filter.Limit, filter.Offset)
	query := photoSelect + " " + where + fmt.Sprintf(" ORDER BY p.taken_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return PhotoPage{}, fmt.Errorf("list photos: %w", err)
	}
	defer rows.Close()
	photos := make([]Photo, 0, filter.Limit)
	for rows.Next() {
		photo, err := scanPhoto(rows)
		if err != nil {
			return PhotoPage{}, err
		}
		photos = append(photos, photo)
	}
	return PhotoPage{Items: photos, Total: total, Limit: filter.Limit, Offset: filter.Offset}, rows.Err()
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Photo, error) {
	photo, err := scanPhoto(s.db.QueryRowContext(ctx, photoSelect+" WHERE p.id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	if err != nil {
		return Photo{}, err
	}
	if err := s.loadPhotoRelations(ctx, &photo); err != nil {
		return Photo{}, fmt.Errorf("load photo relations: %w", err)
	}
	return photo, nil
}

func (s *PostgresStore) Facets(ctx context.Context) (Facets, error) {
	var facets Facets
	queries := []struct {
		query  string
		target *[]FacetCount
	}{
		{`SELECT capture_year::text, count(*) FROM photos GROUP BY capture_year ORDER BY capture_year DESC`, &facets.Years},
		{`SELECT COALESCE(NULLIF(pr.description,''),pr.name,''),count(*) FROM project_photos pp JOIN projects pr ON pr.id=pp.project_id GROUP BY pr.id,pr.description,pr.name ORDER BY count(*) DESC,COALESCE(NULLIF(pr.description,''),pr.name,'')`, &facets.Projects},
		{`SELECT t.name, count(*) FROM photo_tags pt JOIN tags t ON t.id=pt.tag_id GROUP BY t.name ORDER BY count(*) DESC, t.name`, &facets.Tags},
		{`SELECT camera, count(*) FROM photos WHERE camera IS NOT NULL GROUP BY camera ORDER BY count(*) DESC, camera`, &facets.Cameras},
		{`SELECT lens, count(*) FROM photos WHERE lens IS NOT NULL GROUP BY lens ORDER BY count(*) DESC, lens`, &facets.Lenses},
	}
	for _, item := range queries {
		rows, err := s.db.QueryContext(ctx, item.query)
		if err != nil {
			return Facets{}, fmt.Errorf("load facets: %w", err)
		}
		for rows.Next() {
			var value FacetCount
			if err := rows.Scan(&value.Value, &value.Count); err != nil {
				rows.Close()
				return Facets{}, err
			}
			*item.target = append(*item.target, value)
		}
		if err := rows.Close(); err != nil {
			return Facets{}, err
		}
	}
	return facets, nil
}

func (s *PostgresStore) Similar(ctx context.Context, id string, limit int) ([]SimilarPhoto, error) {
	query := `WITH anchor AS (SELECT embedding FROM photos WHERE id=$1 AND embedding IS NOT NULL)
` + strings.Replace(photoSelect, "SELECT ", "SELECT 1 - (p.embedding <=> anchor.embedding) AS similarity, ", 1) + `
CROSS JOIN anchor
WHERE p.id <> $1 AND p.embedding IS NOT NULL
ORDER BY p.embedding <=> anchor.embedding
LIMIT $2`
	rows, err := s.db.QueryContext(ctx, query, id, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar photos: %w", err)
	}
	defer rows.Close()
	result := make([]SimilarPhoto, 0, limit)
	for rows.Next() {
		var similarity float64
		photo, err := scanSimilarPhoto(rows, &similarity)
		if err != nil {
			return nil, err
		}
		result = append(result, SimilarPhoto{Photo: photo, Similarity: similarity})
	}
	if len(result) == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photos WHERE id=$1)`, id).Scan(&exists); err == nil && !exists {
			return nil, ErrNotFound
		}
	}
	return result, rows.Err()
}

func buildWhere(filter Filter) (string, []any) {
	clauses := []string{"TRUE"}
	args := make([]any, 0, 8)
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if filter.Query != "" {
		args = append(args, filter.Query)
		index := len(args)
		clauses = append(clauses, fmt.Sprintf(`(
			p.search_document @@ websearch_to_tsquery('simple', $%d)
			OR p.title ILIKE '%%' || $%d || '%%'
			OR EXISTS (SELECT 1 FROM project_photos qpp JOIN projects qp ON qp.id=qpp.project_id WHERE qpp.photo_id=p.id AND (qp.name ILIKE '%%'||$%d||'%%' OR qp.description ILIKE '%%'||$%d||'%%'))
			OR EXISTS (
				SELECT 1 FROM photo_tags qpt JOIN tags qt ON qt.id=qpt.tag_id
				WHERE qpt.photo_id=p.id AND qt.name ILIKE '%%' || $%d || '%%'
			)
		)`, index, index, index, index, index))
	}
	if filter.Year != 0 {
		add(`p.capture_year = $%d`, filter.Year)
	}
	if filter.Project != "" {
		add(`EXISTS (SELECT 1 FROM project_photos fpp JOIN projects fp ON fp.id=fpp.project_id WHERE fpp.photo_id=p.id AND COALESCE(NULLIF(fp.description,''),fp.name,'')=$%d)`, filter.Project)
	}
	if filter.Camera != "" {
		add(`p.camera = $%d`, filter.Camera)
	}
	if filter.Lens != "" {
		add(`p.lens = $%d`, filter.Lens)
	}
	if filter.MinISO != 0 {
		add(`p.iso >= $%d`, filter.MinISO)
	}
	if filter.MaxISO != 0 {
		add(`p.iso <= $%d`, filter.MaxISO)
	}
	if filter.HasLocation != nil {
		if *filter.HasLocation {
			clauses = append(clauses, `p.latitude IS NOT NULL AND p.longitude IS NOT NULL`)
		} else {
			clauses = append(clauses, `p.latitude IS NULL OR p.longitude IS NULL`)
		}
	}
	for _, tag := range filter.Tags {
		add(`EXISTS (SELECT 1 FROM photo_tags fpt JOIN tags ft ON ft.id=fpt.tag_id WHERE fpt.photo_id=p.id AND ft.name=$%d)`, tag)
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

type scanner interface{ Scan(...any) error }

func scanPhoto(row scanner) (Photo, error) { return scanPhotoFields(row, nil) }
func scanSimilarPhoto(row scanner, similarity *float64) (Photo, error) {
	return scanPhotoFields(row, similarity)
}

func scanPhotoFields(row scanner, similarity *float64) (Photo, error) {
	var photo Photo
	var tagsJSON, metadataJSON, userMetadataJSON, locationName string
	var latitude, longitude sql.NullFloat64
	values := []any{&photo.ID, &photo.Title, &photo.Year, &photo.Project, &photo.TakenAt, &tagsJSON,
		&photo.Camera, &photo.Lens, &photo.Aperture, &photo.ShutterSpeed, &photo.ISO, &photo.FocalLength,
		&photo.Dimensions, &photo.FileType, &photo.FileSize, &locationName, &latitude, &longitude,
		&photo.Path, &photo.ThumbnailPath, &photo.ImageURL, &photo.ThumbnailURL, &photo.AspectRatio, &photo.Dominant, &metadataJSON,
		&photo.Availability, &photo.ThumbnailState, &photo.Description, &photo.Copyright, &photo.Rating, &photo.Favorite,
		&userMetadataJSON, &photo.Revision, &photo.HashStatus, &photo.AIStatus}
	if similarity != nil {
		values = append([]any{similarity}, values...)
	}
	if err := row.Scan(values...); err != nil {
		return Photo{}, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &photo.Tags); err != nil {
		return Photo{}, err
	}
	if err := json.Unmarshal([]byte(metadataJSON), &photo.Metadata); err != nil {
		return Photo{}, err
	}
	if err := json.Unmarshal([]byte(userMetadataJSON), &photo.UserMetadata); err != nil {
		return Photo{}, err
	}
	if latitude.Valid && longitude.Valid {
		photo.Location = &Location{Name: locationName, Latitude: latitude.Float64, Longitude: longitude.Float64}
	}
	return photo, nil
}

func ParseInt(value string) int { parsed, _ := strconv.Atoi(value); return parsed }
