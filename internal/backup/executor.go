package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var ErrBusy = errors.New("another backup operation is already running")

type ExecutorConfig struct {
	Root               string
	StatusPath         string
	ExpectedVolumeUUID string
	PostgresBin        string
	PostgresData       string
	DatabaseURL        string
}

type commandRunner interface {
	Run(context.Context, string, []string, []string) ([]byte, error)
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, name string, args, environment []string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = mergedEnvironment(os.Environ(), environment)
	return command.CombinedOutput()
}

func mergedEnvironment(base, overrides []string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, item := range overrides {
		keys[strings.SplitN(item, "=", 2)[0]] = true
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		if !keys[strings.SplitN(item, "=", 2)[0]] {
			result = append(result, item)
		}
	}
	return append(result, overrides...)
}

type ExecutorOptions struct {
	Now        func() time.Time
	Runner     commandRunner
	VolumeUUID func(context.Context, string) (string, error)
	DiskSpace  func(string) (uint64, uint64, error)
}

type Executor struct {
	config     ExecutorConfig
	database   databaseConfig
	now        func() time.Time
	runner     commandRunner
	volumeUUID func(context.Context, string) (string, error)
	diskSpace  func(string) (uint64, uint64, error)
}

type databaseConfig struct {
	host, port, user, password, database string
}

func NewExecutor(config ExecutorConfig) (*Executor, error) {
	return NewExecutorWithOptions(config, ExecutorOptions{})
}

func NewExecutorWithOptions(config ExecutorConfig, options ExecutorOptions) (*Executor, error) {
	if strings.TrimSpace(config.Root) == "" || strings.TrimSpace(config.StatusPath) == "" || strings.TrimSpace(config.PostgresBin) == "" || strings.TrimSpace(config.PostgresData) == "" || strings.TrimSpace(config.DatabaseURL) == "" {
		return nil, errors.New("backup root, status path, PostgreSQL bin and data directories, and database URL are required")
	}
	database, err := parseDatabaseURL(config.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(filepath.Clean(config.Root), string(filepath.Separator)+"Volumes"+string(filepath.Separator)) && strings.TrimSpace(config.ExpectedVolumeUUID) == "" {
		return nil, errors.New("external backup root requires an expected Volume UUID")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	runner := options.Runner
	if runner == nil {
		runner = systemRunner{}
	}
	volumeUUID := options.VolumeUUID
	if volumeUUID == nil {
		volumeUUID = actualVolumeUUID
	}
	space := options.DiskSpace
	if space == nil {
		space = diskSpace
	}
	return &Executor{config: config, database: database, now: now, runner: runner, volumeUUID: volumeUUID, diskSpace: space}, nil
}

func (e *Executor) RunBackup(ctx context.Context) (State, error) {
	var result State
	err := e.withLock(func() error {
		if err := e.checkRoot(ctx); err != nil {
			return err
		}
		state, _ := readState(e.config.StatusPath)
		now := e.now().UTC()
		state.Version, state.RunningOperation, state.OperationStartedAt = 1, "backup", &now
		state.LastError, state.LastErrorAt = "", nil
		if err := writeState(e.config.StatusPath, state); err != nil {
			return err
		}
		complete := func(operationErr error) {
			state.RunningOperation, state.OperationStartedAt = "", nil
			if operationErr != nil {
				failedAt := e.now().UTC()
				state.LastError, state.LastErrorAt = operationErr.Error(), &failedAt
			}
			_ = writeState(e.config.StatusPath, state)
			result = state
		}

		operationErr := e.createBackup(ctx, &state)
		complete(operationErr)
		return operationErr
	})
	return result, err
}

func (e *Executor) VerifyLatest(ctx context.Context) (State, error) {
	var result State
	err := e.withLock(func() error {
		if err := e.checkRoot(ctx); err != nil {
			return err
		}
		latest, err := latestBackup(e.config.Root)
		if err != nil {
			return err
		}
		state, _ := readState(e.config.StatusPath)
		now := e.now().UTC()
		state.Version, state.RunningOperation, state.OperationStartedAt = 1, "verify", &now
		state.LastError, state.LastErrorAt = "", nil
		if err := writeState(e.config.StatusPath, state); err != nil {
			return err
		}
		operationErr := e.restoreTest(ctx, latest)
		state.RunningOperation, state.OperationStartedAt = "", nil
		if operationErr != nil {
			failedAt := e.now().UTC()
			state.LastError, state.LastErrorAt = operationErr.Error(), &failedAt
		} else {
			verifiedAt := e.now().UTC()
			state.LastVerifiedAt, state.LastVerifiedPath = &verifiedAt, latest
		}
		_ = writeState(e.config.StatusPath, state)
		result = state
		return operationErr
	})
	return result, err
}

func (e *Executor) RunScheduled(ctx context.Context) (State, error) {
	state, err := e.RunBackup(ctx)
	if err != nil {
		return state, err
	}
	if state.LastVerifiedAt == nil || e.now().UTC().Sub(*state.LastVerifiedAt) > 30*24*time.Hour {
		return e.VerifyLatest(ctx)
	}
	return state, nil
}

func (e *Executor) createBackup(ctx context.Context, state *State) error {
	if err := cleanupStalePartials(e.config.Root, e.now().UTC()); err != nil {
		return fmt.Errorf("clean stale partial backups: %w", err)
	}
	databaseBytes, err := e.databaseSize(ctx)
	if err != nil {
		return err
	}
	free, _, err := e.diskSpace(e.config.Root)
	if err != nil {
		return fmt.Errorf("inspect backup disk space: %w", err)
	}
	minimumFree := uint64(databaseBytes) + 1<<30
	if minimumFree < 5<<30 {
		minimumFree = 5 << 30
	}
	if free < minimumFree {
		return fmt.Errorf("backup volume has %d bytes free; at least %d bytes are required", free, minimumFree)
	}
	directory := filepath.Join(e.config.Root, "postgres")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	timestamp := e.now().UTC().Format("20060102T150405Z")
	finalPath := filepath.Join(directory, fmt.Sprintf("apofocus-%s-%d.dump", timestamp, os.Getpid()))
	temporaryPath := finalPath + fmt.Sprintf(".partial-%d", os.Getpid())
	defer os.Remove(temporaryPath)
	args := append(e.database.connectionArgs(e.database.database),
		"--format=custom", "--compress=zstd:6", "--no-owner", "--no-privileges", "--file", temporaryPath)
	if err := e.run(ctx, "pg_dump", args...); err != nil {
		return err
	}
	if err := e.run(ctx, "pg_restore", "--list", temporaryPath); err != nil {
		return fmt.Errorf("validate backup archive: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("protect backup archive: %w", err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("finalize backup archive: %w", err)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return err
	}
	completedAt := e.now().UTC()
	state.LastBackupAt, state.LastBackupPath, state.LastBackupBytes = &completedAt, finalPath, info.Size()
	if err := applyRetention(e.config.Root, completedAt); err != nil {
		return fmt.Errorf("apply backup retention: %w", err)
	}
	return nil
}

func (e *Executor) restoreTest(ctx context.Context, backupPath string) error {
	if err := e.cleanupStaleVerifyDatabases(ctx); err != nil {
		return err
	}
	databaseBytes, err := e.databaseSize(ctx)
	if err != nil {
		return err
	}
	free, _, err := e.diskSpace(e.config.PostgresData)
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL data disk space: %w", err)
	}
	minimumFree := uint64(databaseBytes) + uint64(databaseBytes)/2 + (5 << 30)
	if free < minimumFree {
		return fmt.Errorf("PostgreSQL data volume has %d bytes free; restore testing requires at least %d bytes", free, minimumFree)
	}
	databaseName := fmt.Sprintf("apofocus_verify_%s_%d", e.now().UTC().Format("20060102_150405"), os.Getpid())
	createArgs := []string{"--host", e.database.host, "--port", e.database.port, "--username", e.database.user, "--template", "template0", databaseName}
	if err := e.run(ctx, "createdb", createArgs...); err != nil {
		return err
	}
	dropped := false
	defer func() {
		if !dropped {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			_ = e.run(cleanupContext, "dropdb", "--force", "--host", e.database.host, "--port", e.database.port, "--username", e.database.user, databaseName)
		}
	}()
	restoreArgs := append(e.database.connectionArgs(databaseName), "--exit-on-error", "--no-owner", "--no-privileges", backupPath)
	if err := e.run(ctx, "pg_restore", restoreArgs...); err != nil {
		return err
	}
	query := "SELECT CASE WHEN to_regclass('public.photos') IS NOT NULL AND to_regclass('public.projects') IS NOT NULL AND to_regclass('public.tags') IS NOT NULL THEN 'ok' ELSE 'missing' END"
	queryArgs := append(e.database.connectionArgs(databaseName), "--tuples-only", "--no-align", "--command", query)
	output, err := e.runOutput(ctx, "psql", queryArgs...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != "ok" {
		return errors.New("restore test is missing required catalog relations")
	}
	if err := e.run(ctx, "dropdb", "--force", "--host", e.database.host, "--port", e.database.port, "--username", e.database.user, databaseName); err != nil {
		return err
	}
	dropped = true
	return nil
}

func (e *Executor) cleanupStaleVerifyDatabases(ctx context.Context) error {
	query := "SELECT datname FROM pg_database WHERE starts_with(datname, 'apofocus_verify_') ORDER BY datname"
	args := append(e.database.connectionArgs("postgres"), "--tuples-only", "--no-align", "--command", query)
	output, err := e.runOutput(ctx, "psql", args...)
	if err != nil {
		return fmt.Errorf("list stale restore-test databases: %w", err)
	}
	for _, name := range strings.Fields(string(output)) {
		if !validVerifyDatabaseName(name) {
			return fmt.Errorf("refusing to remove unexpected restore-test database name %q", name)
		}
		if err := e.run(ctx, "dropdb", "--force", "--host", e.database.host, "--port", e.database.port, "--username", e.database.user, name); err != nil {
			return fmt.Errorf("remove stale restore-test database %s: %w", name, err)
		}
	}
	return nil
}

func validVerifyDatabaseName(value string) bool {
	if !strings.HasPrefix(value, "apofocus_verify_") || len(value) > 63 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func (e *Executor) databaseSize(ctx context.Context) (int64, error) {
	args := append(e.database.connectionArgs(e.database.database), "--tuples-only", "--no-align", "--command", "SELECT pg_database_size(current_database())")
	output, err := e.runOutput(ctx, "psql", args...)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse database size: %w", err)
	}
	return value, nil
}

func (e *Executor) checkRoot(ctx context.Context) error {
	info, err := os.Stat(e.config.Root)
	if err != nil {
		return fmt.Errorf("backup root is unavailable: %w", err)
	}
	if !info.IsDir() {
		return errors.New("backup root is not a directory")
	}
	if expected := strings.TrimSpace(e.config.ExpectedVolumeUUID); expected != "" {
		resolvedRoot, err := filepath.EvalSymlinks(e.config.Root)
		if err != nil {
			return fmt.Errorf("resolve backup root: %w", err)
		}
		actual, err := e.volumeUUID(ctx, resolvedRoot)
		if err != nil {
			return fmt.Errorf("identify backup volume: %w", err)
		}
		if !strings.EqualFold(expected, actual) {
			return errors.New("backup volume UUID does not match; refusing to write")
		}
	}
	return nil
}

func (e *Executor) withLock(operation func() error) error {
	lockPath := e.config.StatusPath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ErrBusy
		}
		return err
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return operation()
}

func (e *Executor) run(ctx context.Context, program string, args ...string) error {
	_, err := e.runOutput(ctx, program, args...)
	return err
}

func (e *Executor) runOutput(ctx context.Context, program string, args ...string) ([]byte, error) {
	path := filepath.Join(e.config.PostgresBin, program)
	output, err := e.runner.Run(ctx, path, args, []string{"PGPASSWORD=" + e.database.password, "DATABASE_URL="})
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return output, fmt.Errorf("%s failed: %s", program, detail)
	}
	return output, nil
}

func writeState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := path + fmt.Sprintf(".tmp-%d", os.Getpid())
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func latestBackup(root string) (string, error) {
	directory := filepath.Join(root, "postgres")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read backup directory: %w", err)
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var files []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dump") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		files = append(files, candidate{filepath.Join(directory, entry.Name()), info.ModTime()})
	}
	if len(files) == 0 {
		return "", errors.New("no completed backup is available")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	return files[0].path, nil
}

func applyRetention(root string, now time.Time) error {
	directory := filepath.Join(root, "postgres")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dump") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, file{filepath.Join(directory, entry.Name()), info.ModTime().UTC()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	daily, weekly, monthly := map[string]bool{}, map[string]bool{}, map[string]bool{}
	keep := map[string]bool{}
	for _, item := range files {
		age := now.Sub(item.mod)
		switch {
		case age <= 7*24*time.Hour:
			key := item.mod.Format("2006-01-02")
			if len(daily) < 7 && !daily[key] {
				daily[key], keep[item.path] = true, true
			}
		case age <= 35*24*time.Hour:
			year, week := item.mod.ISOWeek()
			key := fmt.Sprintf("%04d-%02d", year, week)
			if len(weekly) < 4 && !weekly[key] {
				weekly[key], keep[item.path] = true, true
			}
		case age <= 220*24*time.Hour:
			key := item.mod.Format("2006-01")
			if len(monthly) < 6 && !monthly[key] {
				monthly[key], keep[item.path] = true, true
			}
		}
	}
	for _, item := range files {
		if !keep[item.path] {
			if err := os.Remove(item.path); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanupStalePartials(root string, now time.Time) error {
	directory := filepath.Join(root, "postgres")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.Contains(entry.Name(), ".dump.partial-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) > 24*time.Hour {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseDatabaseURL(value string) (databaseConfig, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return databaseConfig{}, fmt.Errorf("parse database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return databaseConfig{}, errors.New("database URL must use postgres or postgresql")
	}
	password, _ := parsed.User.Password()
	database := strings.TrimPrefix(parsed.Path, "/")
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	result := databaseConfig{host: parsed.Hostname(), port: port, user: parsed.User.Username(), password: password, database: database}
	if result.host == "" || result.user == "" || result.database == "" {
		return databaseConfig{}, errors.New("database URL must include host, user, and database")
	}
	return result, nil
}

func (d databaseConfig) connectionArgs(database string) []string {
	return []string{"--host", d.host, "--port", d.port, "--username", d.user, "--dbname", database}
}
