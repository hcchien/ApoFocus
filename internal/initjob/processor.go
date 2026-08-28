package initjob

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hcchien/apofocus/internal/fileidentity"
	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/mediaingest"
	"github.com/hcchien/apofocus/internal/storagewatch"
)

type CatalogProcessor struct {
	db            *sql.DB
	libraryRoot   string
	photoAnalyzer ingest.Analyzer
	mediaAnalyzer mediaingest.Analyzer
	storage       *storagewatch.PostgresRepository
	mu            sync.Mutex
	roots         map[string]storagewatch.Root
}

func NewCatalogProcessor(db *sql.DB, libraryRoot string, photoAnalyzer ingest.Analyzer, mediaAnalyzer mediaingest.Analyzer) *CatalogProcessor {
	return &CatalogProcessor{db: db, libraryRoot: libraryRoot, photoAnalyzer: photoAnalyzer, mediaAnalyzer: mediaAnalyzer, storage: storagewatch.NewPostgresRepository(db), roots: map[string]storagewatch.Root{}}
}

func (p *CatalogProcessor) Catalog(ctx context.Context, run Run, item Item) (string, error) {
	rootPath := catalogStorageRoot(run.SourceRoot, item.SourcePath)
	root, e := p.root(ctx, rootPath)
	if e != nil {
		return "", e
	}
	relative, e := filepath.Rel(root.BasePath, item.SourcePath)
	if e != nil {
		return "", e
	}
	identity, e := fileidentity.FromPath(item.SourcePath)
	if e != nil {
		return "", e
	}
	if item.MediaType == "photo" {
		metadata, e := ingest.ReadFastMetadata(item.SourcePath)
		if e != nil {
			return "", e
		}
		return p.insertPhoto(ctx, run, item, root.ID, filepath.ToSlash(relative), identity.FileID, metadata)
	}
	probe, e := probeMedia(ctx, item.SourcePath, item.MediaType)
	if e != nil {
		return "", e
	}
	return p.insertMedia(ctx, run, item, root.ID, filepath.ToSlash(relative), identity.FileID, probe)
}

func (p *CatalogProcessor) ExcludePath(path string) bool {
	return pathWithin(filepath.Clean(path), filepath.Clean(p.libraryRoot))
}

func catalogStorageRoot(sourceRoot, sourcePath string) string {
	root := filepath.Clean(sourceRoot)
	// A /Volumes scan may cross several independently mounted archives. Keep a
	// distinct root per volume so availability and filesystem events stay useful.
	if root == string(filepath.Separator)+"Volumes" {
		relative, err := filepath.Rel(root, filepath.Clean(sourcePath))
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			parts := strings.Split(relative, string(filepath.Separator))
			if len(parts) > 0 && parts[0] != "" {
				return filepath.Join(root, parts[0])
			}
		}
	}
	return root
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (p *CatalogProcessor) root(ctx context.Context, path string) (storagewatch.Root, error) {
	p.mu.Lock()
	root, ok := p.roots[path]
	p.mu.Unlock()
	if ok {
		return root, nil
	}
	root, e := p.storage.EnsureRoot(ctx, path)
	if e != nil {
		return root, e
	}
	p.mu.Lock()
	p.roots[path] = root
	p.mu.Unlock()
	return root, nil
}
func (p *CatalogProcessor) managedRoot(ctx context.Context) (storagewatch.Root, error) {
	return p.root(ctx, p.libraryRoot)
}

func (p *CatalogProcessor) insertPhoto(ctx context.Context, run Run, item Item, rootID, relative, fileID string, m ingest.Inspection) (string, error) {
	tx, e := p.db.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer func() { _ = tx.Rollback() }()
	projectID, e := upsertProject(ctx, tx, run.Project)
	if e != nil {
		return "", e
	}
	metadata, e := json.Marshal(m.Metadata)
	if e != nil {
		return "", e
	}
	var locationName string
	var lat, lng any
	if m.Location != nil {
		locationName = m.Location.Name
		lat = m.Location.Latitude
		lng = m.Location.Longitude
	}
	var id string
	e = tx.QueryRowContext(ctx, `WITH new_id AS (SELECT gen_random_uuid() id) INSERT INTO photos(id,project_id,title,capture_year,taken_at,camera,lens,aperture,shutter_speed,iso,focal_length,dimensions,file_type,file_size,location_name,latitude,longitude,path,content_sha256,image_url,thumbnail_url,aspect_ratio,metadata,storage_root_id,relative_path,file_id,availability_status,last_verified_at,thumbnail_status,content_hash_status,ai_status)
	SELECT id,$1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,0),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,NULLIF($14,''),$15,$16,$17,NULL,'/api/v1/photos/'||id::text||'/file','', $18,$19::jsonb,$20,$21,NULLIF($22,''),'available',now(),'unknown','pending','pending' FROM new_id
	ON CONFLICT(path) DO UPDATE SET availability_status='available',last_verified_at=now() RETURNING id::text`, projectID, m.Title, m.Year, m.TakenAt, m.Camera, m.Lens, m.Aperture, m.ShutterSpeed, m.ISO, m.FocalLength, m.Dimensions, m.FileType, humanBytes(item.SizeBytes), locationName, lat, lng, item.SourcePath, m.AspectRatio, metadata, rootID, relative, fileID).Scan(&id)
	if e != nil {
		return "", fmt.Errorf("catalog photo: %w", e)
	}
	if e = insertSharedTags(ctx, tx, "photo", id, run.Tags); e != nil {
		return "", e
	}
	if e = tx.Commit(); e != nil {
		return "", e
	}
	return id, nil
}

type mediaProbe struct {
	DurationMS                  int64
	MimeType, Codec, Dimensions string
	SampleRate, Channels        int
	RecordedAt                  time.Time
	Metadata                    map[string]any
}

func (p *CatalogProcessor) insertMedia(ctx context.Context, run Run, item Item, rootID, relative, fileID string, m mediaProbe) (string, error) {
	tx, e := p.db.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer func() { _ = tx.Rollback() }()
	projectID, e := upsertProject(ctx, tx, run.Project)
	if e != nil {
		return "", e
	}
	metadata, e := json.Marshal(m.Metadata)
	if e != nil {
		return "", e
	}
	title := strings.TrimSuffix(filepath.Base(item.SourcePath), filepath.Ext(item.SourcePath))
	var id string
	e = tx.QueryRowContext(ctx, `WITH new_id AS (SELECT gen_random_uuid() id) INSERT INTO media_assets(id,media_type,project_id,title,capture_year,recorded_at,duration_ms,mime_type,codec,dimensions,sample_rate,channels,path,content_sha256,media_url,thumbnail_url,metadata,storage_root_id,relative_path,file_id,availability_status,last_verified_at,thumbnail_status,content_hash_status,ai_status,deep_index_status)
	SELECT id,$1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,0),NULLIF($11,0),$12,NULL,'/api/v1/'||CASE WHEN $1='video' THEN 'videos' ELSE 'audios' END||'/'||id::text||'/file','',$13::jsonb,$14,$15,NULLIF($16,''),'available',now(),'unknown','pending','pending','pending' FROM new_id
	ON CONFLICT(path) DO UPDATE SET availability_status='available',last_verified_at=now() RETURNING id::text`, item.MediaType, projectID, title, m.RecordedAt.Year(), m.RecordedAt, m.DurationMS, m.MimeType, m.Codec, m.Dimensions, m.SampleRate, m.Channels, item.SourcePath, metadata, rootID, relative, fileID).Scan(&id)
	if e != nil {
		return "", fmt.Errorf("catalog media: %w", e)
	}
	if e = insertSharedTags(ctx, tx, "media", id, run.Tags); e != nil {
		return "", e
	}
	if e = tx.Commit(); e != nil {
		return "", e
	}
	return id, nil
}

type preparedPhoto struct {
	item            Item
	hash, thumbnail string
}

func (p *CatalogProcessor) AnalyzePhoto(ctx context.Context, run Run, item Item) error {
	return p.AnalyzePhotoBatch(ctx, run, []Item{item})[item.ID]
}
func (p *CatalogProcessor) AnalyzePhotoBatch(ctx context.Context, run Run, items []Item) map[int64]error {
	results := map[int64]error{}
	prepared := []preparedPhoto{}
	inputs := []ingest.AnalyzeInput{}
	for _, item := range items {
		if item.AssetID == "" {
			results[item.ID] = errors.New("cataloged photo has no asset id")
			continue
		}
		var status string
		if e := p.db.QueryRowContext(ctx, `SELECT ai_status FROM photos WHERE id=$1`, item.AssetID).Scan(&status); e != nil {
			results[item.ID] = e
			continue
		}
		if status == "completed" {
			results[item.ID] = nil
			continue
		}
		_, _ = p.db.ExecContext(ctx, `UPDATE photos SET ai_status='running',content_hash_status='running',updated_at=now() WHERE id=$1`, item.AssetID)
		hash, e := hashFile(item.SourcePath)
		if e != nil {
			results[item.ID] = e
			p.failPhoto(item.AssetID)
			continue
		}
		duplicate, e := p.photoDuplicate(ctx, item.AssetID, hash)
		if e != nil {
			results[item.ID] = e
			p.failPhoto(item.AssetID)
			continue
		}
		if duplicate != "" {
			_, e = p.db.ExecContext(ctx, `UPDATE photos SET duplicate_of=$2,content_hash_status='completed',ai_status='completed',updated_at=now() WHERE id=$1`, item.AssetID, duplicate)
			results[item.ID] = e
			continue
		}
		thumbnail := filepath.Join(p.libraryRoot, "thumbnails", "init", "photos", item.AssetID+".avif")
		if e = os.MkdirAll(filepath.Dir(thumbnail), 0o750); e != nil {
			results[item.ID] = e
			p.failPhoto(item.AssetID)
			continue
		}
		prepared = append(prepared, preparedPhoto{item: item, hash: hash, thumbnail: thumbnail})
		inputs = append(inputs, ingest.AnalyzeInput{Path: item.SourcePath, ThumbnailPath: thumbnail})
	}
	if len(prepared) == 0 {
		return results
	}
	analyses := map[string]ingest.Analysis{}
	if batcher, ok := p.photoAnalyzer.(ingest.BatchAnalyzer); ok {
		batchResults, e := batcher.AnalyzeBatch(ctx, inputs)
		if e != nil {
			for _, entry := range prepared {
				results[entry.item.ID] = e
				p.failPhoto(entry.item.AssetID)
			}
			return results
		}
		for _, entry := range batchResults {
			analyses[entry.Path] = entry.Analysis
		}
	} else {
		for _, entry := range prepared {
			analysis, e := p.photoAnalyzer.Analyze(ctx, entry.item.SourcePath, entry.thumbnail)
			if e != nil {
				results[entry.item.ID] = e
				p.failPhoto(entry.item.AssetID)
				continue
			}
			analyses[entry.item.SourcePath] = analysis
		}
	}
	managed, e := p.managedRoot(ctx)
	if e != nil {
		for _, entry := range prepared {
			results[entry.item.ID] = e
			p.failPhoto(entry.item.AssetID)
		}
		return results
	}
	for _, entry := range prepared {
		if _, exists := results[entry.item.ID]; exists {
			continue
		}
		analysis, ok := analyses[entry.item.SourcePath]
		if !ok {
			e = errors.New("batch analyzer omitted photo")
			results[entry.item.ID] = e
			p.failPhoto(entry.item.AssetID)
			continue
		}
		e = p.finishPhoto(ctx, entry, analysis, managed)
		results[entry.item.ID] = e
		if e != nil {
			p.failPhoto(entry.item.AssetID)
		}
	}
	return results
}
func (p *CatalogProcessor) failPhoto(id string) {
	_, _ = p.db.ExecContext(context.Background(), `UPDATE photos SET ai_status='failed',content_hash_status='failed',updated_at=now() WHERE id=$1`, id)
}
func (p *CatalogProcessor) finishPhoto(ctx context.Context, entry preparedPhoto, analysis ingest.Analysis, managed storagewatch.Root) error {
	identity, e := fileidentity.FromPath(entry.thumbnail)
	if e != nil {
		return e
	}
	relative, _ := filepath.Rel(p.libraryRoot, entry.thumbnail)
	url := "/media/" + filepath.ToSlash(relative)
	tx, e := p.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	_, e = tx.ExecContext(ctx, `UPDATE photos SET content_sha256=$2,content_hash_status='completed',embedding=$3::vector,dominant_color=$4,thumbnail_path=$5,thumbnail_relative_path=$6,thumbnail_file_id=$7,thumbnail_storage_root_id=$8,thumbnail_url=$9,thumbnail_status='available',ai_status='completed',updated_at=now() WHERE id=$1`, entry.item.AssetID, entry.hash, vectorLiteral(analysis.Embedding), analysis.DominantColor, entry.thumbnail, filepath.ToSlash(relative), identity.FileID, managed.ID, url)
	if e != nil {
		return e
	}
	var edited bool
	if e = tx.QueryRowContext(ctx, `SELECT tags_user_edited FROM photos WHERE id=$1`, entry.item.AssetID).Scan(&edited); e != nil {
		return e
	}
	if !edited {
		if e = insertAITags(ctx, tx, "photo", entry.item.AssetID, analysis.Tags, "visual_ai"); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func (p *CatalogProcessor) AnalyzeMedia(ctx context.Context, run Run, item Item) (resultErr error) {
	if item.AssetID == "" {
		return errors.New("cataloged media has no asset id")
	}
	var status string
	if e := p.db.QueryRowContext(ctx, `SELECT ai_status FROM media_assets WHERE id=$1`, item.AssetID).Scan(&status); e != nil {
		return e
	}
	if status == "completed" {
		return nil
	}
	_, _ = p.db.ExecContext(ctx, `UPDATE media_assets SET ai_status='running',content_hash_status='running',deep_index_status='running',updated_at=now() WHERE id=$1`, item.AssetID)
	defer func() {
		if resultErr != nil {
			_, _ = p.db.ExecContext(context.Background(), `UPDATE media_assets SET ai_status='failed',content_hash_status='failed',deep_index_status='failed',updated_at=now() WHERE id=$1`, item.AssetID)
		}
	}()
	hash, e := hashFile(item.SourcePath)
	if e != nil {
		return e
	}
	if duplicate, e := p.mediaDuplicate(ctx, item.AssetID, hash); e != nil {
		return e
	} else if duplicate != "" {
		_, e = p.db.ExecContext(ctx, `UPDATE media_assets SET duplicate_of=$2,content_hash_status='completed',ai_status='completed',deep_index_status='skipped',updated_at=now() WHERE id=$1`, item.AssetID, duplicate)
		return e
	}
	managed, e := p.managedRoot(ctx)
	if e != nil {
		return e
	}
	thumbnail := ""
	if item.MediaType == "video" {
		thumbnail = filepath.Join(p.libraryRoot, "thumbnails", "init", "media", item.AssetID+".avif")
		if e = os.MkdirAll(filepath.Dir(thumbnail), 0o750); e != nil {
			return e
		}
	}
	// Deep-index vectors and transcripts live in PostgreSQL. Current media
	// analysis does not persist keyframes, so avoid creating one empty directory
	// per asset in a very large reference library.
	analysis, e := p.mediaAnalyzer.AnalyzeMedia(ctx, item.SourcePath, thumbnail, "", true)
	if e != nil {
		return e
	}
	tx, e := p.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer func() { _ = tx.Rollback() }()
	thumbRelative, thumbFileID, thumbURL := "", "", ""
	if thumbnail != "" {
		identity, e := fileidentity.FromPath(thumbnail)
		if e != nil {
			return e
		}
		thumbFileID = identity.FileID
		thumbRelative, _ = filepath.Rel(p.libraryRoot, thumbnail)
		thumbRelative = filepath.ToSlash(thumbRelative)
		thumbURL = "/media/" + thumbRelative
	}
	_, e = tx.ExecContext(ctx, `UPDATE media_assets SET content_sha256=$2,content_hash_status='completed',duration_ms=$3,mime_type=$4,codec=$5,dimensions=$6,sample_rate=NULLIF($7,0),channels=NULLIF($8,0),transcript=CASE WHEN transcript_user_edited THEN transcript ELSE $9 END,metadata=$10::jsonb,thumbnail_path=NULLIF($11,''),thumbnail_relative_path=NULLIF($12,''),thumbnail_file_id=NULLIF($13,''),thumbnail_storage_root_id=CASE WHEN $11='' THEN NULL ELSE $14::uuid END,thumbnail_url=$15,thumbnail_status=CASE WHEN $11='' THEN 'unknown' ELSE 'available' END,ai_status='completed',deep_index_status='completed',updated_at=now() WHERE id=$1`, item.AssetID, hash, analysis.DurationMS, analysis.MimeType, analysis.Codec, analysis.Dimensions, analysis.SampleRate, analysis.Channels, analysis.Transcript, mustJSON(analysis.Metadata), thumbnail, thumbRelative, thumbFileID, managed.ID, thumbURL)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM media_segments WHERE media_asset_id=$1`, item.AssetID); e != nil {
		return e
	}
	for _, segment := range analysis.Segments {
		if _, e = tx.ExecContext(ctx, `INSERT INTO media_segments(media_asset_id,segment_index,segment_type,start_ms,end_ms,transcript,tags,visual_embedding,audio_embedding,metadata) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,NULLIF($8,'')::vector,NULLIF($9,'')::vector,$10::jsonb)`, item.AssetID, segment.Index, segment.SegmentType, segment.StartMS, segment.EndMS, segment.Transcript, mustJSON(segment.Tags), vectorLiteral(segment.VisualVector), vectorLiteral(segment.AudioVector), mustJSON(segment.Metadata)); e != nil {
			return e
		}
	}
	var edited bool
	if e = tx.QueryRowContext(ctx, `SELECT tags_user_edited FROM media_assets WHERE id=$1`, item.AssetID).Scan(&edited); e != nil {
		return e
	}
	if !edited {
		source := "audio"
		if item.MediaType == "video" {
			source = "visual"
		}
		if e = insertAITags(ctx, tx, "media", item.AssetID, analysis.Tags, source); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func upsertProject(ctx context.Context, tx *sql.Tx, name string) (any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未分類"
	}
	var id string
	e := tx.QueryRowContext(ctx, `INSERT INTO projects(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, name).Scan(&id)
	return id, e
}
func insertSharedTags(ctx context.Context, tx *sql.Tx, kind, id string, tags []string) error {
	for _, tag := range tags {
		var tagID string
		if e := tx.QueryRowContext(ctx, `INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, tag).Scan(&tagID); e != nil {
			return e
		}
		query := `INSERT INTO photo_tags(photo_id,tag_id,source) VALUES($1,$2,'shared') ON CONFLICT DO NOTHING`
		if kind == "media" {
			query = `INSERT INTO media_asset_tags(media_asset_id,tag_id,source) VALUES($1,$2,'shared') ON CONFLICT DO NOTHING`
		}
		if _, e := tx.ExecContext(ctx, query, id, tagID); e != nil {
			return e
		}
	}
	return nil
}
func insertAITags(ctx context.Context, tx *sql.Tx, kind, id string, tags []string, source string) error {
	for _, tag := range tags {
		var tagID string
		if e := tx.QueryRowContext(ctx, `INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id::text`, tag).Scan(&tagID); e != nil {
			return e
		}
		query := `INSERT INTO photo_tags(photo_id,tag_id,source) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`
		if kind == "media" {
			query = `INSERT INTO media_asset_tags(media_asset_id,tag_id,source) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`
		}
		if _, e := tx.ExecContext(ctx, query, id, tagID, source); e != nil {
			return e
		}
	}
	return nil
}
func (p *CatalogProcessor) photoDuplicate(ctx context.Context, id, hash string) (string, error) {
	var other string
	e := p.db.QueryRowContext(ctx, `SELECT id::text FROM photos WHERE content_sha256=$1 AND id<>$2 LIMIT 1`, hash, id).Scan(&other)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return other, e
}
func (p *CatalogProcessor) mediaDuplicate(ctx context.Context, id, hash string) (string, error) {
	var other string
	e := p.db.QueryRowContext(ctx, `SELECT id::text FROM media_assets WHERE content_sha256=$1 AND id<>$2 LIMIT 1`, hash, id).Scan(&other)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return other, e
}
func hashFile(path string) (string, error) {
	file, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer file.Close()
	hash := sha256.New()
	if _, e = io.Copy(hash, file); e != nil {
		return "", e
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func vectorLiteral(values []float32) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%.8f", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
func mustJSON(value any) string { data, _ := json.Marshal(value); return string(data) }
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

func probeMedia(ctx context.Context, path, expected string) (mediaProbe, error) {
	binary, e := exec.LookPath("ffprobe")
	if e != nil {
		return mediaProbe{}, errors.New("ffprobe is required for fast catalog")
	}
	command := exec.CommandContext(ctx, binary, "-v", "error", "-show_format", "-show_streams", "-of", "json", path)
	output, e := command.Output()
	if e != nil {
		return mediaProbe{}, fmt.Errorf("ffprobe %s: %w", path, e)
	}
	var raw struct {
		Streams []map[string]any `json:"streams"`
		Format  map[string]any   `json:"format"`
	}
	if e = json.Unmarshal(output, &raw); e != nil {
		return mediaProbe{}, e
	}
	// Do not persist a second copy of the absolute source path inside ffprobe
	// metadata. The canonical path is already stored in the catalog row.
	delete(raw.Format, "filename")
	var video, audio map[string]any
	for _, stream := range raw.Streams {
		kind, _ := stream["codec_type"].(string)
		if kind == "video" && video == nil {
			video = stream
		}
		if kind == "audio" && audio == nil {
			audio = stream
		}
	}
	detected := "audio"
	if video != nil {
		detected = "video"
	}
	if detected != expected {
		return mediaProbe{}, fmt.Errorf("extension suggests %s but ffprobe detected %s", expected, detected)
	}
	primary := audio
	if video != nil {
		primary = video
	}
	durationSeconds, _ := strconv.ParseFloat(text(raw.Format["duration"]), 64)
	recorded := time.Now()
	if info, e := os.Stat(path); e == nil {
		recorded = info.ModTime()
	}
	if tags, ok := raw.Format["tags"].(map[string]any); ok {
		if value := text(tags["creation_time"]); value != "" {
			if parsed, e := time.Parse(time.RFC3339, value); e == nil {
				recorded = parsed
			}
		}
	}
	width, height := intNumber(primary["width"]), intNumber(primary["height"])
	dimensions := ""
	if width > 0 && height > 0 {
		dimensions = fmt.Sprintf("%d × %d", width, height)
	}
	return mediaProbe{DurationMS: int64(durationSeconds * 1000), MimeType: mime.TypeByExtension(strings.ToLower(filepath.Ext(path))), Codec: text(primary["codec_name"]), Dimensions: dimensions, SampleRate: intNumber(audioValue(audio, "sample_rate")), Channels: intNumber(audioValue(audio, "channels")), RecordedAt: recorded, Metadata: map[string]any{"ffprobe": raw}}, nil
}
func text(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func intNumber(value any) int {
	number, _ := strconv.Atoi(strings.Split(text(value), ".")[0])
	return number
}
func audioValue(audio map[string]any, key string) any {
	if audio == nil {
		return nil
	}
	return audio[key]
}
