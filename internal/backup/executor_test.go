package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	commands []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args, environment []string) ([]byte, error) {
	command := filepath.Base(name) + " " + strings.Join(args, " ")
	f.commands = append(f.commands, command)
	for _, arg := range args {
		if strings.Contains(arg, "secret-password") {
			return nil, errors.New("password leaked into process arguments")
		}
	}
	switch filepath.Base(name) {
	case "psql":
		if strings.Contains(command, "pg_database_size") {
			return []byte("1024\n"), nil
		}
		if strings.Contains(command, "starts_with(datname, apofocus_verify_)") || strings.Contains(command, "starts_with(datname, 'apofocus_verify_')") {
			return nil, nil
		}
		return []byte("ok\n"), nil
	case "pg_dump":
		for index, arg := range args {
			if arg == "--file" && index+1 < len(args) {
				return nil, os.WriteFile(args[index+1], []byte("valid custom archive"), 0o600)
			}
		}
		return nil, errors.New("pg_dump file argument missing")
	default:
		return nil, nil
	}
}

func TestExecutorCreatesAndRestoreTestsBackup(t *testing.T) {
	root, statePath := t.TempDir(), filepath.Join(t.TempDir(), "backup-status.json")
	runner := &fakeRunner{}
	now := time.Now().UTC().Truncate(time.Second)
	executor, err := NewExecutorWithOptions(ExecutorConfig{
		Root: root, StatusPath: statePath, ExpectedVolumeUUID: "volume-1", PostgresBin: "/postgres/bin", PostgresData: root,
		DatabaseURL: "postgresql://apofocus:secret-password@127.0.0.1:55432/apofocus?sslmode=disable",
	}, ExecutorOptions{
		Now:        func() time.Time { return now },
		Runner:     runner,
		VolumeUUID: func(context.Context, string) (string, error) { return "volume-1", nil },
		DiskSpace:  func(string) (uint64, uint64, error) { return 20 << 30, 100 << 30, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := executor.RunScheduled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.LastBackupAt == nil || state.LastVerifiedAt == nil || state.RunningOperation != "" || state.LastError != "" {
		t.Fatalf("unexpected completed state: %+v", state)
	}
	if _, err := os.Stat(state.LastBackupPath); err != nil {
		t.Fatalf("completed backup does not exist: %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, expected := range []string{"pg_dump ", "pg_restore --list", "createdb ", "pg_restore --host", "dropdb --force"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("missing command %q in:\n%s", expected, joined)
		}
	}
}

func TestExecutorRejectsWrongVolumeBeforeWriting(t *testing.T) {
	executor, err := NewExecutorWithOptions(ExecutorConfig{
		Root: t.TempDir(), StatusPath: filepath.Join(t.TempDir(), "status.json"), ExpectedVolumeUUID: "expected",
		PostgresBin: "/postgres/bin", PostgresData: t.TempDir(), DatabaseURL: "postgresql://user:password@127.0.0.1:5432/apofocus",
	}, ExecutorOptions{
		Runner:     &fakeRunner{},
		VolumeUUID: func(context.Context, string) (string, error) { return "different", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunBackup(context.Background()); err == nil || !strings.Contains(err.Error(), "UUID does not match") {
		t.Fatalf("expected UUID mismatch, got %v", err)
	}
}

func TestRetentionKeepsNewestDailyAndRemovesExpiredArchives(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "postgres")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	create := func(name string, age time.Duration) string {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := now.Add(-age)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		return path
	}
	newest := create("apofocus-newest.dump", time.Hour)
	olderSameDay := create("apofocus-older.dump", 2*time.Hour)
	expired := create("apofocus-expired.dump", 221*24*time.Hour)
	partial := create("apofocus-crashed.dump.partial-42", 25*time.Hour)
	if err := cleanupStalePartials(root, now); err != nil {
		t.Fatal(err)
	}
	if err := applyRetention(root, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("newest daily backup was removed: %v", err)
	}
	for _, removed := range []string{olderSameDay, expired, partial} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s to be removed, got %v", removed, err)
		}
	}
}

func TestVerifyRefusesToFillPostgresDataVolume(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "postgres")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "latest.dump"), []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	executor, err := NewExecutorWithOptions(ExecutorConfig{
		Root: root, StatusPath: filepath.Join(t.TempDir(), "status.json"), PostgresBin: "/postgres/bin", PostgresData: t.TempDir(),
		DatabaseURL: "postgresql://user:password@127.0.0.1:5432/apofocus",
	}, ExecutorOptions{Runner: runner, DiskSpace: func(string) (uint64, uint64, error) { return 1 << 20, 1 << 30, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.VerifyLatest(context.Background()); err == nil || !strings.Contains(err.Error(), "restore testing requires") {
		t.Fatalf("expected restore disk guard, got %v", err)
	}
	for _, command := range runner.commands {
		if strings.HasPrefix(command, "createdb ") {
			t.Fatalf("created a restore database despite low space: %s", command)
		}
	}
}
