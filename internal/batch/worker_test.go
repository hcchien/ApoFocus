package batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/mediaingest"
)

type workerRepo struct {
	job       Job
	claimed   bool
	items     []Item
	completed int
	failed    int
	finished  bool
}

func (r *workerRepo) Create(context.Context, CreateInput) (Job, error)   { return Job{}, nil }
func (r *workerRepo) Get(context.Context, string) (Job, error)           { return r.job, nil }
func (r *workerRepo) Items(context.Context, string, int) ([]Item, error) { return r.items, nil }
func (r *workerRepo) Cancel(context.Context, string) error               { return nil }
func (r *workerRepo) ClaimNext(context.Context) (Job, bool, error) {
	if r.claimed {
		return Job{}, false, nil
	}
	r.claimed = true
	return r.job, true, nil
}
func (r *workerRepo) AddDiscovered(_ context.Context, _ string, files []DiscoveredFile) error {
	for _, file := range files {
		r.items = append(r.items, Item{ID: int64(len(r.items) + 1), SourcePath: file.Path, MediaType: file.MediaType, Status: "pending"})
	}
	return nil
}
func (r *workerRepo) StartRunning(context.Context, string) error { return nil }
func (r *workerRepo) NextItem(_ context.Context, _ string) (Item, bool, error) {
	for index := range r.items {
		if r.items[index].Status == "pending" {
			r.items[index].Status = "running"
			return r.items[index], true, nil
		}
	}
	return Item{}, false, nil
}
func (r *workerRepo) CompleteItem(_ context.Context, _ string, id int64, _, _ string, err error) error {
	r.items[id-1].Status = "succeeded"
	r.completed++
	if err != nil {
		r.items[id-1].Status = "failed"
		r.failed++
	}
	return nil
}
func (r *workerRepo) Finish(context.Context, string, error) error             { r.finished = true; return nil }
func (r *workerRepo) Heartbeat(context.Context, string, string) (bool, error) { return false, nil }

type sequentialImporter struct {
	mu        sync.Mutex
	active    int
	maxActive int
	paths     []string
}

type sequentialMediaImporter struct{ paths []string }

func (i *sequentialMediaImporter) Import(_ context.Context, request mediaingest.ImportRequest) (mediaingest.ImportResult, error) {
	i.paths = append(i.paths, request.SourcePath)
	return mediaingest.ImportResult{AssetID: "media"}, nil
}

func (i *sequentialImporter) ValidateBatchRoot(path string) (string, error) { return path, nil }
func (i *sequentialImporter) Import(_ context.Context, request ingest.ImportRequest) (ingest.ImportResult, error) {
	i.mu.Lock()
	i.active++
	if i.active > i.maxActive {
		i.maxActive = i.active
	}
	i.paths = append(i.paths, request.SourcePath)
	i.active--
	i.mu.Unlock()
	if filepath.Base(request.SourcePath) == "bad.jpg" {
		return ingest.ImportResult{}, errors.New("decode failed")
	}
	return ingest.ImportResult{PhotoID: "photo"}, nil
}

func TestWorkerScansRecursivelyAndProcessesSequentially(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "one.jpg"))
	mustWrite(t, filepath.Join(root, "clip.mov"))
	mustWrite(t, filepath.Join(root, "sound.wav"))
	mustWrite(t, filepath.Join(root, "notes.txt"))
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "nested", "bad.jpg"))
	if err := os.MkdirAll(filepath.Join(root, ".hidden"), 0o750); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".hidden", "ignored.jpg"))
	repo := &workerRepo{job: Job{ID: "job", SourceRoot: root, Recursive: true, AutoTags: true, MediaTypes: []string{"photo", "video", "audio"}}}
	importer := &sequentialImporter{}
	mediaImporter := &sequentialMediaImporter{}
	worker := NewWorker(repo, importer, mediaImporter)
	if err := worker.runOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 4 {
		t.Fatalf("expected 4 discovered media files, got %d", len(repo.items))
	}
	if repo.completed != 4 || repo.failed != 1 || !repo.finished {
		t.Fatalf("unexpected progress: %+v", repo)
	}
	if importer.maxActive != 1 {
		t.Fatalf("expected sequential processing, max active=%d", importer.maxActive)
	}
	if len(mediaImporter.paths) != 2 {
		t.Fatalf("expected video and audio imports, got %v", mediaImporter.paths)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("photo"), 0o640); err != nil {
		t.Fatal(err)
	}
}
