package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const mediaSelect = `
SELECT m.id::text, m.media_type, m.title, m.capture_year, COALESCE(pr.name, ''), m.recorded_at,
       m.duration_ms, m.mime_type, m.codec, m.dimensions, COALESCE(m.sample_rate, 0), COALESCE(m.channels, 0),
       COALESCE((SELECT json_agg(t.name ORDER BY t.name) FROM media_asset_tags mat JOIN tags t ON t.id=mat.tag_id WHERE mat.media_asset_id=m.id), '[]')::text,
       m.path, COALESCE(m.thumbnail_path, ''), m.media_url, m.thumbnail_url, m.transcript,
       COALESCE(m.metadata, '{}'::jsonb)::text,
       COALESCE(m.availability_status, 'unknown'), COALESCE(m.thumbnail_status, 'unknown'),
       COALESCE(m.description,''),COALESCE(m.copyright,''),COALESCE(m.rating,0),COALESCE(m.favorite,false),
       COALESCE(m.user_metadata,'{}'::jsonb)::text,COALESCE(m.revision,1),COALESCE(m.content_hash_status,'completed'),
       COALESCE(m.ai_status,'completed'),COALESCE(m.deep_index_status,'completed')
FROM media_assets m
LEFT JOIN projects pr ON pr.id=m.project_id`

func (s *PostgresStore) ListMedia(ctx context.Context, filter MediaFilter) (MediaPage, error) {
	where, args := buildMediaWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM media_assets m LEFT JOIN projects pr ON pr.id=m.project_id `+where, args...).Scan(&total); err != nil {
		return MediaPage{}, fmt.Errorf("count media: %w", err)
	}
	args = append(args, filter.Limit, filter.Offset)
	query := mediaSelect + " " + where + fmt.Sprintf(" ORDER BY m.recorded_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return MediaPage{}, fmt.Errorf("list media: %w", err)
	}
	defer rows.Close()
	items := make([]MediaAsset, 0, filter.Limit)
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return MediaPage{}, err
		}
		item.Transcript = ""
		item.Metadata = nil
		items = append(items, item)
	}
	return MediaPage{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, rows.Err()
}

func (s *PostgresStore) GetMedia(ctx context.Context, mediaType, id string) (MediaAsset, error) {
	asset, err := scanMedia(s.db.QueryRowContext(ctx, mediaSelect+` WHERE m.media_type=$1 AND m.id=$2`, mediaType, id))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaAsset{}, ErrNotFound
	}
	if err != nil {
		return MediaAsset{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id::text,segment_type,segment_index,start_ms,end_ms,keyframe_url,transcript,tags::text,metadata::text,COALESCE(keyframe_status,'unknown')
		FROM media_segments WHERE media_asset_id=$1 ORDER BY start_ms,segment_type,segment_index`, id)
	if err != nil {
		return MediaAsset{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var segment MediaSegment
		var tagsJSON, metadataJSON string
		if err := rows.Scan(&segment.ID, &segment.SegmentType, &segment.Index, &segment.StartMS, &segment.EndMS, &segment.KeyframeURL, &segment.Transcript, &tagsJSON, &metadataJSON, &segment.KeyframeState); err != nil {
			return MediaAsset{}, err
		}
		if err := json.Unmarshal([]byte(tagsJSON), &segment.Tags); err != nil {
			return MediaAsset{}, err
		}
		if err := json.Unmarshal([]byte(metadataJSON), &segment.Metadata); err != nil {
			return MediaAsset{}, err
		}
		asset.Segments = append(asset.Segments, segment)
	}
	return asset, rows.Err()
}

func (s *PostgresStore) MediaFacets(ctx context.Context, mediaType string) (MediaFacets, error) {
	var facets MediaFacets
	queries := []struct {
		query  string
		target *[]FacetCount
	}{
		{`SELECT capture_year::text,count(*) FROM media_assets WHERE media_type=$1 GROUP BY capture_year ORDER BY capture_year DESC`, &facets.Years},
		{`SELECT pr.name,count(*) FROM media_assets m JOIN projects pr ON pr.id=m.project_id WHERE m.media_type=$1 GROUP BY pr.name ORDER BY count(*) DESC,pr.name`, &facets.Projects},
		{`SELECT t.name,count(*) FROM media_asset_tags mat JOIN media_assets m ON m.id=mat.media_asset_id JOIN tags t ON t.id=mat.tag_id WHERE m.media_type=$1 GROUP BY t.name ORDER BY count(*) DESC,t.name`, &facets.Tags},
		{`SELECT codec,count(*) FROM media_assets WHERE media_type=$1 AND codec<>'' GROUP BY codec ORDER BY count(*) DESC,codec`, &facets.Codecs},
	}
	for _, item := range queries {
		rows, err := s.db.QueryContext(ctx, item.query, mediaType)
		if err != nil {
			return MediaFacets{}, fmt.Errorf("load media facets: %w", err)
		}
		for rows.Next() {
			var value FacetCount
			if err := rows.Scan(&value.Value, &value.Count); err != nil {
				rows.Close()
				return MediaFacets{}, err
			}
			*item.target = append(*item.target, value)
		}
		if err := rows.Close(); err != nil {
			return MediaFacets{}, err
		}
	}
	return facets, nil
}

func (s *PostgresStore) SimilarMedia(ctx context.Context, mediaType, id, modality string, limit int) ([]SimilarMedia, error) {
	column := "audio_embedding"
	if modality == "visual" {
		column = "visual_embedding"
	}
	query := fmt.Sprintf(`WITH anchor AS (
		SELECT %s AS embedding FROM media_segments WHERE media_asset_id=$1 AND %s IS NOT NULL
	), ranked AS (
		SELECT candidate.media_asset_id, 1-MIN(candidate.%s <=> anchor.embedding) AS similarity
		FROM media_segments candidate CROSS JOIN anchor
		JOIN media_assets ma ON ma.id=candidate.media_asset_id
		WHERE candidate.media_asset_id<>$1 AND candidate.%s IS NOT NULL AND ma.media_type=$2
		GROUP BY candidate.media_asset_id ORDER BY MIN(candidate.%s <=> anchor.embedding) LIMIT $3
	) SELECT media_asset_id::text,similarity FROM ranked ORDER BY similarity DESC`, column, column, column, column, column)
	rows, err := s.db.QueryContext(ctx, query, id, mediaType, limit)
	if err != nil {
		return nil, fmt.Errorf("find similar media: %w", err)
	}
	defer rows.Close()
	type ranked struct {
		id         string
		similarity float64
	}
	rankedItems := []ranked{}
	for rows.Next() {
		var item ranked
		if err := rows.Scan(&item.id, &item.similarity); err != nil {
			return nil, err
		}
		rankedItems = append(rankedItems, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(rankedItems) == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets WHERE id=$1 AND media_type=$2)`, id, mediaType).Scan(&exists); err == nil && !exists {
			return nil, ErrNotFound
		}
	}
	result := make([]SimilarMedia, 0, len(rankedItems))
	for _, item := range rankedItems {
		asset, err := scanMedia(s.db.QueryRowContext(ctx, mediaSelect+` WHERE m.id=$1`, item.id))
		if err != nil {
			return nil, err
		}
		asset.Transcript = ""
		asset.Metadata = nil
		result = append(result, SimilarMedia{Asset: asset, Similarity: item.similarity})
	}
	return result, nil
}

func buildMediaWhere(filter MediaFilter) (string, []any) {
	clauses := []string{"m.media_type=$1"}
	args := []any{filter.MediaType}
	add := func(format string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if filter.Query != "" {
		args = append(args, filter.Query)
		i := len(args)
		clauses = append(clauses, fmt.Sprintf(`(m.search_document @@ websearch_to_tsquery('simple',$%d) OR m.title ILIKE '%%'||$%d||'%%' OR pr.name ILIKE '%%'||$%d||'%%' OR EXISTS (SELECT 1 FROM media_asset_tags qmat JOIN tags qt ON qt.id=qmat.tag_id WHERE qmat.media_asset_id=m.id AND qt.name ILIKE '%%'||$%d||'%%'))`, i, i, i, i))
	}
	if filter.Year != 0 {
		add(`m.capture_year=$%d`, filter.Year)
	}
	if filter.Project != "" {
		add(`pr.name=$%d`, filter.Project)
	}
	if filter.Codec != "" {
		add(`m.codec=$%d`, filter.Codec)
	}
	if filter.MinDuration > 0 {
		add(`m.duration_ms >= $%d`, filter.MinDuration)
	}
	if filter.MaxDuration > 0 {
		add(`m.duration_ms <= $%d`, filter.MaxDuration)
	}
	if filter.HasTranscript != nil {
		if *filter.HasTranscript {
			clauses = append(clauses, `m.transcript<>''`)
		} else {
			clauses = append(clauses, `m.transcript=''`)
		}
	}
	for _, tag := range filter.Tags {
		add(`EXISTS (SELECT 1 FROM media_asset_tags fmat JOIN tags ft ON ft.id=fmat.tag_id WHERE fmat.media_asset_id=m.id AND ft.name=$%d)`, tag)
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanMedia(row scanner) (MediaAsset, error) {
	var asset MediaAsset
	var tagsJSON, metadataJSON, userMetadataJSON string
	if err := row.Scan(&asset.ID, &asset.MediaType, &asset.Title, &asset.Year, &asset.Project, &asset.RecordedAt,
		&asset.DurationMS, &asset.MimeType, &asset.Codec, &asset.Dimensions, &asset.SampleRate, &asset.Channels,
		&tagsJSON, &asset.Path, &asset.ThumbnailPath, &asset.MediaURL, &asset.ThumbnailURL, &asset.Transcript, &metadataJSON,
		&asset.Availability, &asset.ThumbnailState, &asset.Description, &asset.Copyright, &asset.Rating, &asset.Favorite,
		&userMetadataJSON, &asset.Revision, &asset.HashStatus, &asset.AIStatus, &asset.DeepIndexState); err != nil {
		return MediaAsset{}, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &asset.Tags); err != nil {
		return MediaAsset{}, err
	}
	if err := json.Unmarshal([]byte(metadataJSON), &asset.Metadata); err != nil {
		return MediaAsset{}, err
	}
	if err := json.Unmarshal([]byte(userMetadataJSON), &asset.UserMetadata); err != nil {
		return MediaAsset{}, err
	}
	return asset, nil
}
