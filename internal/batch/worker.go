package batch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/mediaingest"
)

type Worker struct {
	repository    Repository
	importer      Importer
	mediaImporter MediaImporter
	pollEvery     time.Duration
}

func NewWorker(repository Repository, importer Importer, mediaImporters ...MediaImporter) *Worker {
	worker := &Worker{repository: repository, importer: importer, pollEvery: time.Second}
	if len(mediaImporters) > 0 {
		worker.mediaImporter = mediaImporters[0]
	}
	return worker
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		if err := w.runOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			// Job-level failures are persisted by runOne; keep the local worker alive.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) runOne(ctx context.Context) error {
	job, found, err := w.repository.ClaimNext(ctx)
	if err != nil || !found {
		return err
	}
	if err := w.scanAndPersist(ctx, job); errors.Is(err, errCancelRequested) {
		return w.repository.Finish(ctx, job.ID, nil)
	} else if err != nil {
		_ = w.repository.Finish(ctx, job.ID, err)
		return err
	}
	if err := w.repository.StartRunning(ctx, job.ID); err != nil {
		_ = w.repository.Finish(ctx, job.ID, err)
		return err
	}
	for {
		cancelled, err := w.repository.Heartbeat(ctx, job.ID, "")
		if err != nil {
			_ = w.repository.Finish(ctx, job.ID, err)
			return err
		}
		if cancelled {
			return w.repository.Finish(ctx, job.ID, nil)
		}
		item, found, err := w.repository.NextItem(ctx, job.ID)
		if err != nil {
			_ = w.repository.Finish(ctx, job.ID, err)
			return err
		}
		if !found {
			break
		}
		if _, err := w.repository.Heartbeat(ctx, job.ID, item.SourcePath); err != nil {
			return err
		}
		assetID := ""
		var importErr error
		if item.MediaType == "photo" {
			result, err := w.importer.Import(ctx, ingest.ImportRequest{SourcePath: item.SourcePath, Project: job.Project, Tags: job.Tags, AutoTags: job.AutoTags})
			assetID, importErr = result.PhotoID, err
		} else if w.mediaImporter == nil {
			importErr = errors.New("video/audio importer is not configured")
		} else {
			result, err := w.mediaImporter.Import(ctx, mediaingest.ImportRequest{SourcePath: item.SourcePath, Project: job.Project, Tags: job.Tags, AutoTags: job.AutoTags})
			assetID, importErr = result.AssetID, err
		}
		if err := w.repository.CompleteItem(ctx, job.ID, item.ID, item.MediaType, assetID, importErr); err != nil {
			_ = w.repository.Finish(ctx, job.ID, err)
			return err
		}
	}
	return w.repository.Finish(ctx, job.ID, nil)
}

var errCancelRequested = errors.New("batch cancellation requested")

func (w *Worker) scanAndPersist(ctx context.Context, job Job) error {
	pending := make([]DiscoveredFile, 0, 250)
	allowed := map[string]bool{}
	for _, mediaType := range job.MediaTypes {
		allowed[mediaType] = true
	}
	visited := 0
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		sort.Slice(pending, func(i, j int) bool { return pending[i].Path < pending[j].Path })
		if err := w.repository.AddDiscovered(ctx, job.ID, pending); err != nil {
			return err
		}
		pending = pending[:0]
		cancelled, err := w.repository.Heartbeat(ctx, job.ID, "掃描資料夾中")
		if err != nil {
			return err
		}
		if cancelled {
			return errCancelRequested
		}
		return nil
	}
	visit := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		visited++
		if visited%1000 == 0 {
			cancelled, heartbeatErr := w.repository.Heartbeat(ctx, job.ID, path)
			if heartbeatErr != nil {
				return heartbeatErr
			}
			if cancelled {
				return errCancelRequested
			}
		}
		if path != job.SourceRoot && entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			mediaType := detectedType(path)
			if mediaType != "" && allowed[mediaType] {
				pending = append(pending, DiscoveredFile{Path: path, MediaType: mediaType})
			}
		}
		if len(pending) == cap(pending) {
			return flush()
		}
		return nil
	}
	if job.Recursive {
		if err := filepath.WalkDir(job.SourceRoot, visit); err != nil {
			return err
		}
	} else {
		entries, err := os.ReadDir(job.SourceRoot)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := visit(filepath.Join(job.SourceRoot, entry.Name()), entry, nil); err != nil {
				return err
			}
		}
	}
	return flush()
}

var photoExtensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".tif": true, ".tiff": true, ".webp": true, ".heic": true, ".heif": true, ".dng": true, ".arw": true, ".cr2": true, ".cr3": true, ".nef": true, ".raf": true}

func detectedType(path string) string {
	if photoExtensions[strings.ToLower(filepath.Ext(path))] {
		return "photo"
	}
	return mediaingest.DetectMediaType(path)
}
