package storagewatch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Supervisor discovers storage roots registered by later init runs and keeps
// one fsnotify watcher per mounted root. This lets reference libraries retain
// move/missing-file tracking without requiring a Web service restart.
type Supervisor struct {
	repository *PostgresRepository
	logger     *slog.Logger
	mu         sync.Mutex
	running    map[string]context.CancelFunc
}

func NewSupervisor(repository *PostgresRepository, logger *slog.Logger) *Supervisor {
	return &Supervisor{repository: repository, logger: logger, running: map[string]context.CancelFunc{}}
}
func (s *Supervisor) Run(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.sync(ctx); err != nil {
			s.logger.Error("sync storage watchers", "error", err)
		}
		select {
		case <-ctx.Done():
			s.stopAll()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (s *Supervisor) sync(ctx context.Context) error {
	roots, err := s.repository.ListRoots(ctx)
	if err != nil {
		return err
	}
	for _, root := range roots {
		if _, err := os.Stat(root.BasePath); err != nil {
			continue
		}
		s.mu.Lock()
		_, exists := s.running[root.ID]
		s.mu.Unlock()
		if exists {
			continue
		}
		watcher, err := NewWatcher(root, s.repository, s.logger)
		if err != nil {
			continue
		}
		watchCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.running[root.ID] = cancel
		s.mu.Unlock()
		go func(root Root) {
			err := watcher.Run(watchCtx)
			if err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Error("filesystem watcher stopped", "path", root.BasePath, "error", err)
			}
			s.mu.Lock()
			delete(s.running, root.ID)
			s.mu.Unlock()
		}(root)
		s.logger.Info("filesystem watcher enabled", "path", root.BasePath)
	}
	return nil
}
func (s *Supervisor) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cancel := range s.running {
		cancel()
	}
	s.running = map[string]context.CancelFunc{}
}
