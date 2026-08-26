package storagewatch

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hcchien/apofocus/internal/fileidentity"
)

type Watcher struct {
	root    Root
	tracker Tracker
	logger  *slog.Logger
	watcher *fsnotify.Watcher
	offline bool
}

func NewWatcher(root Root, tracker Tracker, logger *slog.Logger) (*Watcher, error) {
	watcher, err := fsnotify.NewBufferedWatcher(4096)
	if err != nil {
		return nil, err
	}
	result := &Watcher{root: root, tracker: tracker, logger: logger, watcher: watcher}
	if err := result.addDirectories(root.BasePath); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	return result, nil
}

func (w *Watcher) Close() error { return w.watcher.Close() }

func (w *Watcher) Run(ctx context.Context) error {
	defer w.Close()
	healthTicker := time.NewTicker(3 * time.Second)
	defer healthTicker.Stop()
	pending := map[string]fsnotify.Op{}
	var timer *time.Timer
	var timerChannel <-chan time.Time
	flush := func() {
		for path, operation := range pending {
			if err := w.handle(ctx, path, operation); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Warn("reconcile filesystem event", "path", path, "operation", operation.String(), "error", err)
			}
		}
		clear(pending)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			if w.ignored(event.Name) || event.Op == fsnotify.Chmod {
				continue
			}
			pending[filepath.Clean(event.Name)] |= event.Op
			if timer == nil {
				timer = time.NewTimer(350 * time.Millisecond)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(350 * time.Millisecond)
			}
			timerChannel = timer.C
		case <-timerChannel:
			flush()
			timerChannel = nil
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("filesystem watcher error", "error", err)
			if verifyErr := w.tracker.VerifyKnownPaths(ctx, w.root); verifyErr != nil {
				w.logger.Warn("verify known storage paths after watcher error", "error", verifyErr)
			}
		case <-healthTicker.C:
			w.checkRoot(ctx)
		}
	}
}

func (w *Watcher) handle(ctx context.Context, path string, operation fsnotify.Op) error {
	if operation.Has(fsnotify.Remove) || operation.Has(fsnotify.Rename) {
		if filepath.Clean(path) == filepath.Clean(w.root.BasePath) {
			w.offline = true
			return w.tracker.MarkRootOffline(ctx, w.root)
		}
		if err := w.tracker.MarkMissing(ctx, w.root, path); err != nil {
			return err
		}
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := w.addDirectories(path); err != nil {
			return err
		}
		return w.reconcileTree(ctx, path)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	identity, err := fileidentity.FromPath(path)
	if err != nil {
		return err
	}
	return w.tracker.ObservePath(ctx, w.root, path, identity)
}

func (w *Watcher) checkRoot(ctx context.Context) {
	_, err := os.Stat(w.root.BasePath)
	if errors.Is(err, os.ErrNotExist) {
		if !w.offline {
			w.offline = true
			if markErr := w.tracker.MarkRootOffline(ctx, w.root); markErr != nil {
				w.logger.Warn("mark storage root offline", "error", markErr)
			}
		}
		return
	}
	if err != nil {
		w.logger.Warn("check storage root", "error", err)
		return
	}
	if !w.offline {
		return
	}
	if err := w.addDirectories(w.root.BasePath); err != nil {
		w.logger.Warn("resume storage root watcher", "error", err)
		return
	}
	if err := w.tracker.VerifyKnownPaths(ctx, w.root); err != nil {
		w.logger.Warn("verify remounted storage root", "error", err)
		return
	}
	if err := w.tracker.TouchRoot(ctx, w.root); err != nil {
		w.logger.Warn("mark storage root online", "error", err)
		return
	}
	w.offline = false
	w.logger.Info("managed library remounted", "path", w.root.BasePath)
}

func (w *Watcher) addDirectories(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && w.ignored(path) {
			return filepath.SkipDir
		}
		return w.watcher.Add(path)
	})
}

func (w *Watcher) reconcileTree(ctx context.Context, root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && entry.IsDir() && w.ignored(path) {
			return filepath.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		identity, err := fileidentity.FromPath(path)
		if err != nil {
			return err
		}
		return w.tracker.ObservePath(ctx, w.root, path, identity)
	})
}

func (w *Watcher) ignored(path string) bool {
	relative, err := filepath.Rel(w.root.BasePath, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}
	return false
}
