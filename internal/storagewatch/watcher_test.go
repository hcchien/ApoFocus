package storagewatch

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hcchien/apofocus/internal/fileidentity"
)

type trackerEvent struct {
	kind     string
	path     string
	identity fileidentity.Identity
}

type recordingTracker struct{ events chan trackerEvent }

func (r *recordingTracker) ObservePath(_ context.Context, _ Root, path string, identity fileidentity.Identity) error {
	r.events <- trackerEvent{kind: "observed", path: path, identity: identity}
	return nil
}

func (r *recordingTracker) MarkMissing(_ context.Context, _ Root, path string) error {
	r.events <- trackerEvent{kind: "missing", path: path}
	return nil
}

func (r *recordingTracker) MarkRootOffline(_ context.Context, root Root) error {
	r.events <- trackerEvent{kind: "root_offline", path: root.BasePath}
	return nil
}

func (r *recordingTracker) VerifyKnownPaths(_ context.Context, root Root) error {
	r.events <- trackerEvent{kind: "verified", path: root.BasePath}
	return nil
}

func (r *recordingTracker) TouchRoot(_ context.Context, root Root) error {
	r.events <- trackerEvent{kind: "root_online", path: root.BasePath}
	return nil
}

func TestWatcherObservesCreateAndRename(t *testing.T) {
	rootPath := t.TempDir()
	root := Root{ID: "root", BasePath: rootPath, Status: "online"}
	tracker := &recordingTracker{events: make(chan trackerEvent, 32)}
	watcher, err := NewWatcher(root, tracker, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- watcher.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("watcher did not stop")
		}
	})

	original := filepath.Join(rootPath, "original.jpg")
	if err := os.WriteFile(original, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	created := waitForEvent(t, tracker.events, "observed", original)
	if created.identity.FileID == "" {
		t.Fatal("created file did not have a stable file ID")
	}

	renamed := filepath.Join(rootPath, "renamed.jpg")
	if err := os.Rename(original, renamed); err != nil {
		t.Fatal(err)
	}
	var moved trackerEvent
	missingSeen, movedSeen := false, false
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !missingSeen || !movedSeen {
		select {
		case event := <-tracker.events:
			missingSeen = missingSeen || event.kind == "missing" && event.path == original
			if event.kind == "observed" && event.path == renamed {
				moved = event
				movedSeen = true
			}
		case <-timer.C:
			t.Fatalf("rename events incomplete: missing=%t moved=%t", missingSeen, movedSeen)
		}
	}
	if moved.identity.FileID != created.identity.FileID {
		t.Fatalf("file ID changed across rename: %q != %q", moved.identity.FileID, created.identity.FileID)
	}

	if err := os.RemoveAll(rootPath); err != nil {
		t.Fatal(err)
	}
	waitForEvent(t, tracker.events, "root_offline", rootPath)
	if err := os.MkdirAll(rootPath, 0o700); err != nil {
		t.Fatal(err)
	}
	verified, online := false, false
	remountTimer := time.NewTimer(7 * time.Second)
	defer remountTimer.Stop()
	for !verified || !online {
		select {
		case event := <-tracker.events:
			verified = verified || event.kind == "verified" && event.path == rootPath
			online = online || event.kind == "root_online" && event.path == rootPath
		case <-remountTimer.C:
			t.Fatalf("remount events incomplete: verified=%t online=%t", verified, online)
		}
	}
}

func waitForEvent(t *testing.T, events <-chan trackerEvent, kind, path string) trackerEvent {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.kind == kind && event.path == path {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s event for %s", kind, path)
		}
	}
}
