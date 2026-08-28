package initjob

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hcchien/apofocus/internal/fileidentity"
)

type Worker struct {
	repository     Repository
	processor      Processor
	owner          string
	catalogWorkers int
	pollEvery      time.Duration
	aiReadiness    func(context.Context) error
}

func NewWorker(repository Repository, processor Processor, catalogWorkers int) *Worker {
	if catalogWorkers < 1 {
		catalogWorkers = 2
	}
	return &Worker{repository: repository, processor: processor, owner: workerID(), catalogWorkers: catalogWorkers, pollEvery: time.Second}
}

// SetAIReadiness defers only AI stages until the local model service is ready.
// Discovery and Fast Catalog remain available while a large model is loading
// or temporarily offline.
func (w *Worker) SetAIReadiness(check func(context.Context) error) { w.aiReadiness = check }
func workerID() string {
	host, _ := os.Hostname()
	return host + ":" + strings.TrimSpace(os.Getenv("APOFOCUS_WORKER_ID")) + ":" + time.Now().Format("150405.000")
}
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		if e := w.runOne(ctx); e != nil && !errors.Is(e, context.Canceled) {
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w *Worker) runOne(ctx context.Context) error {
	run, found, e := w.repository.ClaimRun(ctx, w.owner)
	if e != nil || !found {
		return e
	}
	if !run.DiscoveryComplete {
		if e = w.scan(ctx, run); e != nil {
			return w.failOrPause(ctx, run, e)
		}
	}
	if e = w.repository.SetRunStage(ctx, run.ID, "cataloging"); e != nil {
		return e
	}
	if e = w.process(ctx, run, "catalog", "", w.catalogWorkers); e != nil {
		return w.failOrPause(ctx, run, e)
	}
	latest, e := w.repository.Get(ctx, run.ID)
	if e != nil {
		return w.failOrPause(ctx, run, e)
	}
	if latest.CatalogedCount == 0 {
		return w.repository.Finish(ctx, run.ID, nil)
	}
	if e = w.waitForAI(ctx, run); e != nil {
		return w.failOrPause(ctx, run, e)
	}
	if e = w.repository.SetRunStage(ctx, run.ID, "photo_ai"); e != nil {
		return e
	}
	if e = w.process(ctx, run, "ai", "photo", 1); e != nil {
		return w.failOrPause(ctx, run, e)
	}
	if e = w.repository.SetRunStage(ctx, run.ID, "media_ai"); e != nil {
		return e
	}
	if e = w.process(ctx, run, "ai", "media", 1); e != nil {
		return w.failOrPause(ctx, run, e)
	}
	return w.repository.Finish(ctx, run.ID, nil)
}

func (w *Worker) waitForAI(ctx context.Context, run Run) error {
	if w.aiReadiness == nil {
		return nil
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := w.aiReadiness(checkCtx)
		cancel()
		if err == nil {
			return nil
		}
		if err = w.instruction(ctx, run.ID, "waiting_for_embedding"); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var errPause = errors.New("init pause requested")
var errCancel = errors.New("init cancel requested")

func (w *Worker) instruction(ctx context.Context, runID, path string) error {
	instruction, e := w.repository.Heartbeat(ctx, runID, path)
	if e != nil {
		return e
	}
	if instruction == "pause" {
		return errPause
	}
	if instruction == "cancel" {
		return errCancel
	}
	return nil
}
func (w *Worker) failOrPause(ctx context.Context, run Run, e error) error {
	if errors.Is(e, errPause) {
		return w.repository.SetRunStage(ctx, run.ID, "paused")
	}
	if errors.Is(e, errCancel) {
		return w.repository.Finish(ctx, run.ID, nil)
	}
	_ = w.repository.Finish(ctx, run.ID, e)
	return e
}
func (w *Worker) scan(ctx context.Context, run Run) error {
	pending := make([]Discovered, 0, 500)
	visited := 0
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if e := w.repository.AddDiscovered(ctx, run.ID, pending); e != nil {
			return e
		}
		pending = pending[:0]
		return w.instruction(ctx, run.ID, "scanning")
	}
	visit := func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if excluder, ok := w.processor.(PathExcluder); ok && excluder.ExcludePath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != run.SourceRoot && entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		mediaType := detectMedia(path)
		if mediaType == "" {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		identity, _ := fileidentity.FromPath(path)
		pending = append(pending, Discovered{Path: path, MediaType: mediaType, SizeBytes: info.Size(), ModifiedAt: info.ModTime(), FileID: identity.FileID})
		visited++
		if visited%1000 == 0 {
			if e = w.instruction(ctx, run.ID, path); e != nil {
				return e
			}
		}
		if len(pending) == cap(pending) {
			return flush()
		}
		return nil
	}
	if run.Recursive {
		if e := filepath.WalkDir(run.SourceRoot, visit); e != nil {
			return e
		}
	} else {
		entries, e := os.ReadDir(run.SourceRoot)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if e = visit(filepath.Join(run.SourceRoot, entry.Name()), entry, nil); e != nil {
				return e
			}
		}
	}
	if e := flush(); e != nil {
		return e
	}
	return w.repository.SetRunStage(ctx, run.ID, "cataloging")
}
func (w *Worker) process(ctx context.Context, run Run, stage, mediaType string, parallel int) error {
	stageCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsOut := make(chan error, parallel)
	var group sync.WaitGroup
	for index := 0; index < parallel; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				if e := w.instruction(stageCtx, run.ID, stage); e != nil {
					errorsOut <- e
					cancel()
					return
				}
				limit := 1
				if stage == "ai" && mediaType == "photo" {
					limit = 8
				}
				items, e := w.repository.ClaimItems(stageCtx, run.ID, stage, mediaType, limit, w.owner)
				if e != nil {
					errorsOut <- e
					cancel()
					return
				}
				if len(items) == 0 {
					return
				}
				if stage == "catalog" {
					for _, item := range items {
						assetID, itemErr := w.processor.Catalog(stageCtx, run, item)
						if e = w.repository.CompleteCatalog(stageCtx, run.ID, item, assetID, itemErr); e != nil {
							errorsOut <- e
							cancel()
							return
						}
						if itemErr != nil {
							select {
							case <-stageCtx.Done():
								return
							case <-time.After(2 * time.Second):
							}
						}
					}
				} else if mediaType == "photo" {
					results := map[int64]error{}
					if batcher, ok := w.processor.(PhotoBatchProcessor); ok {
						results = batcher.AnalyzePhotoBatch(stageCtx, run, items)
					} else {
						for _, item := range items {
							results[item.ID] = w.processor.AnalyzePhoto(stageCtx, run, item)
						}
					}
					for _, item := range items {
						if e = w.repository.CompleteAI(stageCtx, run.ID, item, results[item.ID]); e != nil {
							errorsOut <- e
							cancel()
							return
						}
						if results[item.ID] != nil {
							select {
							case <-stageCtx.Done():
								return
							case <-time.After(2 * time.Second):
							}
						}
					}
				} else {
					for _, item := range items {
						itemErr := w.processor.AnalyzeMedia(stageCtx, run, item)
						if e = w.repository.CompleteAI(stageCtx, run.ID, item, itemErr); e != nil {
							errorsOut <- e
							cancel()
							return
						}
						if itemErr != nil {
							select {
							case <-stageCtx.Done():
								return
							case <-time.After(2 * time.Second):
							}
						}
					}
				}
			}
		}()
	}
	group.Wait()
	close(errorsOut)
	for e := range errorsOut {
		if e != nil {
			return e
		}
	}
	return nil
}

var photoExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".tif": true, ".tiff": true, ".webp": true, ".heic": true, ".heif": true, ".dng": true, ".arw": true, ".cr2": true, ".cr3": true, ".nef": true, ".raf": true, ".3fr": true, ".orf": true, ".rw2": true, ".pef": true}
var videoExt = map[string]bool{".mp4": true, ".mov": true, ".m4v": true, ".mkv": true, ".avi": true, ".webm": true, ".mts": true, ".m2ts": true}
var audioExt = map[string]bool{".wav": true, ".mp3": true, ".m4a": true, ".aac": true, ".flac": true, ".ogg": true, ".opus": true, ".aiff": true, ".aif": true}

func detectMedia(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if photoExt[ext] {
		return "photo"
	}
	if videoExt[ext] {
		return "video"
	}
	if audioExt[ext] {
		return "audio"
	}
	return ""
}
func DefaultCatalogWorkers() int {
	workers := runtime.NumCPU() / 2
	if workers < 2 {
		workers = 2
	}
	if workers > 4 {
		workers = 4
	}
	return workers
}
