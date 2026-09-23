package deepanalysis

import (
	"context"
	"errors"
	"time"
)

type Worker struct {
	repository   Repository
	analyzer     Analyzer
	pollInterval time.Duration
}

func NewWorker(repository Repository, analyzer Analyzer) *Worker {
	return &Worker{repository: repository, analyzer: analyzer, pollInterval: time.Second}
}

func (w *Worker) Run(ctx context.Context) error {
	_ = w.repository.RecoverStalled(ctx)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		job, ok, err := w.repository.ClaimNext(ctx)
		if err != nil {
			return err
		}
		if ok {
			if err := w.runJob(ctx, job); err != nil && !errors.Is(err, context.Canceled) {
				_ = w.repository.Finish(context.Background(), job.ID, err)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) runJob(ctx context.Context, job Job) error {
	for {
		cancel, err := w.repository.Heartbeat(ctx, job.ID)
		if err != nil {
			return err
		}
		if cancel {
			return w.repository.Finish(ctx, job.ID, nil)
		}
		item, ok, err := w.repository.NextItem(ctx, job.ID)
		if err != nil {
			return err
		}
		if !ok {
			return w.repository.Finish(ctx, job.ID, nil)
		}
		result, analyzeErr := w.analyzer.Analyze(ctx, item.Path)
		if err := w.repository.CompleteItem(ctx, job, item, result, analyzeErr); err != nil {
			return err
		}
	}
}
