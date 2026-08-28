package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hcchien/apofocus/internal/backup"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	executor, err := backup.NewExecutor(backup.ExecutorConfig{
		Root: required("APOFOCUS_BACKUP_ROOT"), StatusPath: required("APOFOCUS_BACKUP_STATUS"),
		ExpectedVolumeUUID: strings.TrimSpace(os.Getenv("APOFOCUS_BACKUP_VOLUME_UUID")),
		PostgresBin:        required("POSTGRES_BIN"), PostgresData: required("POSTGRES_DATA"), DatabaseURL: required("DATABASE_URL"),
	})
	if err != nil {
		logger.Error("configure backup", "error", err)
		os.Exit(1)
	}
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalContext, 24*time.Hour)
	defer cancel()
	var state backup.State
	switch command := firstArg(); command {
	case "run":
		state, err = executor.RunBackup(ctx)
	case "verify":
		state, err = executor.VerifyLatest(ctx)
	case "scheduled":
		state, err = executor.RunScheduled(ctx)
	default:
		fmt.Fprintln(os.Stderr, "Usage: apofocus-backup {run|verify|scheduled}")
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, backup.ErrBusy) {
			logger.Info("backup operation already running")
			return
		}
		logger.Error("backup operation failed", "error", err)
		os.Exit(1)
	}
	output, _ := json.Marshal(state)
	fmt.Println(string(output))
}

func required(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", name)
		os.Exit(2)
	}
	return value
}

func firstArg() string {
	if len(os.Args) < 2 {
		return ""
	}
	return os.Args[1]
}
