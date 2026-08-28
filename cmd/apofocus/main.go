package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
	"github.com/hcchien/apofocus/internal/httpapi"
	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/initjob"
	"github.com/hcchien/apofocus/internal/mediaingest"
	"github.com/hcchien/apofocus/internal/storagewatch"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	store, db, closeStore, err := buildStore(logger)
	if err != nil {
		logger.Error("initialize catalog", "error", err)
		os.Exit(1)
	}
	defer closeStore()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	addr := envOr("ADDR", ":8080")
	mediaRoot := os.Getenv("PHOTO_LIBRARY_ROOT")
	options := httpapi.Options{MediaRoot: mediaRoot}
	if db != nil {
		options.Media = store.(catalog.MediaStore)
		options.Folders = folders.NewPostgresRepository(db)
		storageRootID := ""
		libraryOnline := false
		if mediaRoot != "" {
			storageRepository := storagewatch.NewPostgresRepository(db)
			root, rootErr := storageRepository.EnsureRoot(ctx, mediaRoot)
			if errors.Is(rootErr, storagewatch.ErrRootOffline) {
				logger.Warn("managed library is offline", "path", mediaRoot)
			} else if rootErr != nil {
				logger.Error("configure managed library", "error", rootErr)
				os.Exit(1)
			} else {
				mediaRoot = root.BasePath
				options.MediaRoot = root.BasePath
				storageRootID = root.ID
				libraryOnline = true
				watcher := storagewatch.NewSupervisor(storageRepository, logger)
				go func() {
					if watchErr := watcher.Run(ctx); watchErr != nil && !errors.Is(watchErr, context.Canceled) {
						logger.Error("filesystem watcher stopped", "error", watchErr)
					}
				}()
				logger.Info("storage watcher supervisor enabled")
			}
		}
		if rootsValue := os.Getenv("APOFOCUS_IMPORT_ROOTS"); libraryOnline && rootsValue != "" {
			analyzerURL := envOr("EMBEDDING_SERVICE_URL", "http://127.0.0.1:8090")
			manager, managerErr := ingest.NewManager(mediaRoot, splitPaths(rootsValue), ingest.NewHTTPAnalyzer(analyzerURL), ingest.NewPostgresRepository(db, storageRootID))
			if managerErr != nil {
				logger.Error("configure batch importer", "error", managerErr)
				os.Exit(1)
			}
			mediaManager, mediaManagerErr := mediaingest.NewManager(mediaRoot, splitPaths(rootsValue), mediaingest.NewHTTPAnalyzer(analyzerURL), mediaingest.NewPostgresRepository(db, storageRootID))
			if mediaManagerErr != nil {
				logger.Error("configure video/audio importer", "error", mediaManagerErr)
				os.Exit(1)
			}
			batchRepository := batch.NewPostgresRepository(db)
			initRepository := initjob.NewPostgresRepository(db)
			options.BatchJobs = batch.NewService(batchRepository, manager)
			options.InitJobs = initjob.NewService(initRepository, splitPaths(rootsValue))
			worker := batch.NewWorker(batchRepository, manager, mediaManager)
			go func() {
				if workerErr := worker.Run(ctx); workerErr != nil && !errors.Is(workerErr, context.Canceled) {
					logger.Error("batch worker stopped", "error", workerErr)
				}
			}()
			logger.Info("local sequential batch worker enabled")
		}
	}
	handler := httpapi.NewWithOptions(store, logger, options)
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("ApoFocus is ready", "address", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}

func buildStore(logger *slog.Logger) (catalog.Store, *sql.DB, func(), error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Info("DATABASE_URL is not set; using the built-in demo catalog")
		return catalog.NewMemoryStore(), nil, func() {}, nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, nil, func() {}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, func() {}, err
	}
	logger.Info("connected to PostgreSQL")
	return catalog.NewPostgresStore(db), db, func() { _ = db.Close() }, nil
}

func splitPaths(value string) []string {
	parts := strings.Split(value, string(filepath.ListSeparator))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}
	return result
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
