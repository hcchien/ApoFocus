package mediaingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeAnalyzer struct{ calls int }

func (a *fakeAnalyzer) AnalyzeMedia(_ context.Context, source, thumbnail, segmentDir string, _ bool) (Analysis, error) {
	a.calls++
	mediaType := DetectMediaType(source)
	if thumbnail != "" {
		if err := os.WriteFile(thumbnail, []byte("thumbnail"), 0o640); err != nil {
			return Analysis{}, err
		}
	}
	if mediaType == "audio" {
		audio := make([]float32, 512)
		audio[0] = 1
		return Analysis{
			MediaType: "audio", DurationMS: 6100, MimeType: "audio/wav", Codec: "pcm_s16le",
			RecordedAt: "2026-08-26T12:00:00Z", Tags: []string{"訪談"},
			Segments: []Segment{{SegmentType: "audio", Index: 0, EndMS: 6100, Tags: []string{"訪談"}, AudioVector: audio}},
		}, nil
	}
	if err := os.MkdirAll(segmentDir, 0o750); err != nil {
		return Analysis{}, err
	}
	frame := filepath.Join(segmentDir, "frame-000000.jpg")
	if err := os.WriteFile(frame, []byte("frame"), 0o640); err != nil {
		return Analysis{}, err
	}
	visual := make([]float32, 512)
	visual[0] = 1
	return Analysis{
		MediaType: "video", DurationMS: 6100, MimeType: "video/mp4", Codec: "h264", Dimensions: "640 × 360",
		RecordedAt: "2026-08-26T12:00:00Z", Tags: []string{"紀實", "共同"},
		Segments: []Segment{{SegmentType: "visual", Index: 0, EndMS: 6100, KeyframePath: frame, Tags: []string{"紀實"}, VisualVector: visual}},
	}, nil
}

type fakeRepository struct {
	record Record
	hash   string
}

func (r *fakeRepository) FindByHash(_ context.Context, hash string) (ExistingMedia, bool, error) {
	if r.hash == hash {
		return ExistingMedia{ID: "asset-1", MediaType: r.record.MediaType, Path: r.record.Path, ThumbnailPath: r.record.ThumbnailPath, Tags: r.record.Tags}, true, nil
	}
	return ExistingMedia{}, false, nil
}

func (r *fakeRepository) Insert(_ context.Context, record Record) (string, error) {
	r.record = record
	r.hash = record.ContentSHA256
	return "asset-1", nil
}

func TestManagerImportsVideoAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	library := filepath.Join(root, "library")
	if err := os.MkdirAll(inbox, 0o750); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(inbox, "clip.mp4")
	if err := os.WriteFile(source, []byte("video bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	analyzer := &fakeAnalyzer{}
	repository := &fakeRepository{}
	manager, err := NewManager(library, []string{inbox}, analyzer, repository)
	if err != nil {
		t.Fatal(err)
	}

	result, err := manager.Import(context.Background(), ImportRequest{SourcePath: source, Project: "島嶼 日常", Tags: []string{"共同", "客戶"}, AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyExists || result.MediaType != "video" || result.SegmentCount != 1 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	for _, path := range []string{repository.record.Path, repository.record.ThumbnailPath, repository.record.Segments[0].KeyframePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected managed file %s: %v", path, err)
		}
	}
	if !strings.Contains(repository.record.Path, filepath.Join("originals", "videos", "2026", "島嶼-日常")) {
		t.Fatalf("unexpected managed path: %s", repository.record.Path)
	}
	if filepath.Ext(repository.record.ThumbnailPath) != ".avif" {
		t.Fatalf("thumbnail should use AVIF: %s", repository.record.ThumbnailPath)
	}
	if strings.Join(repository.record.Tags, ",") != "共同,客戶,紀實" {
		t.Fatalf("unexpected merged tags: %v", repository.record.Tags)
	}
	if analyzer.calls != 1 {
		t.Fatalf("expected one analyzer call, got %d", analyzer.calls)
	}

	retry, err := manager.Import(context.Background(), ImportRequest{SourcePath: source, AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	if !retry.AlreadyExists || retry.AssetID != "asset-1" || analyzer.calls != 1 {
		t.Fatalf("retry was not deduplicated: %+v, analyzer calls=%d", retry, analyzer.calls)
	}
}

func TestManagerInspectsVideoWithoutPersistingArtifacts(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	library := filepath.Join(root, "library")
	if err := os.MkdirAll(inbox, 0o750); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(inbox, "preview.mp4")
	if err := os.WriteFile(source, []byte("video bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(library, []string{inbox}, &fakeAnalyzer{}, &fakeRepository{})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := manager.Inspect(context.Background(), ImportRequest{SourcePath: source, Project: "紀錄片", Tags: []string{"客戶"}, AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.MediaType != "video" || inspection.SegmentCount != 1 || inspection.VisualVectorCount != 1 {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
	if !strings.Contains(inspection.SuggestedFolder, filepath.Join("originals", "videos", "2026", "紀錄片")) {
		t.Fatalf("unexpected suggested folder: %s", inspection.SuggestedFolder)
	}
	entries, err := os.ReadDir(filepath.Join(library, ".staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("inspection left staging artifacts: %v", entries)
	}
}

func TestManagerImportsAudioWithoutThumbnail(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	library := filepath.Join(root, "library")
	if err := os.MkdirAll(inbox, 0o750); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(inbox, "interview.wav")
	if err := os.WriteFile(source, []byte("audio bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	repository := &fakeRepository{}
	manager, err := NewManager(library, []string{inbox}, &fakeAnalyzer{}, repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Import(context.Background(), ImportRequest{SourcePath: source, AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.MediaType != "audio" || result.ThumbnailPath != "" || repository.record.ThumbnailURL != "" || repository.record.ThumbnailFileID != "" {
		t.Fatalf("audio should not have a thumbnail artifact: result=%+v record=%+v", result, repository.record)
	}
	if _, err := os.Stat(filepath.Join(library, "thumbnails", "audios")); !os.IsNotExist(err) {
		t.Fatalf("audio import should not create a thumbnail directory: %v", err)
	}
}

func TestManagerReplacesArtifactsLeftByInterruptedImport(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	library := filepath.Join(root, "library")
	if err := os.MkdirAll(inbox, 0o750); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(inbox, "clip.mp4")
	if err := os.WriteFile(source, []byte("video bytes"), 0o640); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(library, []string{inbox}, &fakeAnalyzer{}, &fakeRepository{})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := hashFile(source)
	if err != nil {
		t.Fatal(err)
	}
	recordedAt, _ := time.Parse(time.RFC3339Nano, "2026-08-26T12:00:00Z")
	_, thumbnailPath, segmentDir := manager.destinationPaths("video", "紀錄片", "clip", hash, ".mp4", recordedAt)
	if err := os.MkdirAll(segmentDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumbnailPath, []byte("stale thumbnail"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(segmentDir, "stale.jpg"), []byte("stale"), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := manager.Import(context.Background(), ImportRequest{SourcePath: source, Project: "紀錄片", AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	thumbnail, err := os.ReadFile(result.ThumbnailPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(thumbnail) != "thumbnail" {
		t.Fatalf("stale thumbnail was not replaced: %q", thumbnail)
	}
	if _, err := os.Stat(filepath.Join(segmentDir, "stale.jpg")); !os.IsNotExist(err) {
		t.Fatalf("stale segment survived retry: %v", err)
	}
}

func TestManagerRejectsSourceOutsideAllowlist(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	outside := filepath.Join(root, "outside.mp4")
	if err := os.MkdirAll(inbox, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("video"), 0o640); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(filepath.Join(root, "library"), []string{inbox}, &fakeAnalyzer{}, &fakeRepository{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Import(context.Background(), ImportRequest{SourcePath: outside}); err == nil || !strings.Contains(err.Error(), "outside APOFOCUS_IMPORT_ROOTS") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}
