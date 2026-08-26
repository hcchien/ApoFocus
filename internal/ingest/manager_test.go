package ingest

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeAnalyzer struct {
	calls int
}

func (a *fakeAnalyzer) Analyze(_ context.Context, _ string, thumbnailPath string) (Analysis, error) {
	a.calls++
	if thumbnailPath != "" {
		if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o750); err != nil {
			return Analysis{}, err
		}
		if err := os.WriteFile(thumbnailPath, []byte("thumbnail"), 0o640); err != nil {
			return Analysis{}, err
		}
	}
	return Analysis{Tags: []string{"街頭", "人物"}, Embedding: make([]float32, 512), DominantColor: "#776655"}, nil
}

type fakeRepository struct {
	existing *ExistingPhoto
	record   PhotoRecord
}

func (r *fakeRepository) FindByHash(_ context.Context, _ string) (ExistingPhoto, bool, error) {
	if r.existing != nil {
		return *r.existing, true, nil
	}
	return ExistingPhoto{}, false, nil
}

func (r *fakeRepository) Insert(_ context.Context, record PhotoRecord) (string, error) {
	r.record = record
	return "photo-123", nil
}

func TestInspectPlansFolderAndMergesTags(t *testing.T) {
	inbox, library := t.TempDir(), t.TempDir()
	photo := createJPEG(t, inbox, "DSC_0001.jpg")
	takenAt := time.Date(2024, 5, 3, 14, 30, 0, 0, time.Local)
	if err := os.Chtimes(photo, takenAt, takenAt); err != nil {
		t.Fatal(err)
	}
	analyzer := &fakeAnalyzer{}
	manager, err := NewManager(library, []string{inbox}, analyzer, nil)
	if err != nil {
		t.Fatal(err)
	}

	inspection, err := manager.Inspect(context.Background(), ImportRequest{
		SourcePath: photo, Project: "城市 日常", Tags: []string{"精選"}, AutoTags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Year != 2024 {
		t.Fatalf("expected year 2024, got %d", inspection.Year)
	}
	if !strings.Contains(inspection.SuggestedFolder, filepath.Join("originals", "2024", "城市-日常")) {
		t.Fatalf("unexpected folder: %s", inspection.SuggestedFolder)
	}
	if strings.Join(inspection.SuggestedTags, ",") != "人物,精選,街頭" {
		t.Fatalf("unexpected tags: %v", inspection.SuggestedTags)
	}
	if len(inspection.ContentSHA256) != 64 {
		t.Fatalf("unexpected SHA-256: %s", inspection.ContentSHA256)
	}
}

func TestInspectRejectsPathOutsideAllowlist(t *testing.T) {
	inbox, library, outside := t.TempDir(), t.TempDir(), t.TempDir()
	photo := createJPEG(t, outside, "outside.jpg")
	manager, err := NewManager(library, []string{inbox}, &fakeAnalyzer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Inspect(context.Background(), ImportRequest{SourcePath: photo, AutoTags: false})
	if err == nil || !strings.Contains(err.Error(), "outside APOFOCUS_IMPORT_ROOTS") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestImportCopiesAnalyzesAndCreatesRecord(t *testing.T) {
	inbox, library := t.TempDir(), t.TempDir()
	photo := createJPEG(t, inbox, "source.jpg")
	analyzer, repository := &fakeAnalyzer{}, &fakeRepository{}
	manager, err := NewManager(library, []string{inbox}, analyzer, repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Import(context.Background(), ImportRequest{
		SourcePath: photo, Title: "午後街角", Project: "島嶼日常", Tags: []string{"委託案"}, AutoTags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PhotoID != "photo-123" || result.VectorDimensions != 512 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("copied photo missing: %v", err)
	}
	if _, err := os.Stat(result.ThumbnailPath); err != nil {
		t.Fatalf("thumbnail missing: %v", err)
	}
	if repository.record.ImageURL == "" || repository.record.ThumbnailURL == "" {
		t.Fatal("media URLs were not assigned")
	}
	if analyzer.calls != 1 {
		t.Fatalf("expected one model call, got %d", analyzer.calls)
	}
}

func TestImportIsIdempotentByHash(t *testing.T) {
	inbox, library := t.TempDir(), t.TempDir()
	photo := createJPEG(t, inbox, "source.jpg")
	analyzer := &fakeAnalyzer{}
	repository := &fakeRepository{existing: &ExistingPhoto{ID: "existing", Path: "/library/photo.jpg", Tags: []string{"既有"}}}
	manager, err := NewManager(library, []string{inbox}, analyzer, repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Import(context.Background(), ImportRequest{SourcePath: photo, AutoTags: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyExists || result.PhotoID != "existing" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if analyzer.calls != 0 {
		t.Fatal("duplicate should not invoke the embedding model")
	}
}

func createJPEG(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	imageData := image.NewRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			imageData.Set(x, y, color.RGBA{R: 120, G: 80, B: 40, A: 255})
		}
	}
	if err := jpeg.Encode(file, imageData, &jpeg.Options{Quality: 80}); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
