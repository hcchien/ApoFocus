package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/initjob"
	"github.com/hcchien/apofocus/internal/mediaingest"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	databaseURL := required("DATABASE_URL")
	libraryRoot := required("PHOTO_LIBRARY_ROOT")
	embeddingURL := envOr("EMBEDDING_SERVICE_URL", "http://127.0.0.1:8090")
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		logger.Error("open PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		logger.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	repository := initjob.NewPostgresRepository(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	processor := initjob.NewCatalogProcessor(db, libraryRoot, ingest.NewHTTPAnalyzer(embeddingURL), mediaingest.NewHTTPAnalyzer(embeddingURL))
	workers := initjob.DefaultCatalogWorkers()
	if value, parseErr := strconv.Atoi(os.Getenv("APOFOCUS_CATALOG_WORKERS")); parseErr == nil && value > 0 {
		workers = value
	}
	worker := initjob.NewWorker(repository, processor, workers)
	worker.SetAIReadiness(func(checkCtx context.Context) error { return checkEmbedding(checkCtx, embeddingURL) })
	logger.Info("ApoFocus init worker ready", "catalog_workers", workers, "photo_ai_workers", 1, "media_ai_workers", 1)
	if err = worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("init worker stopped", "error", err)
		os.Exit(1)
	}
}

func checkEmbedding(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New(response.Status)
	}
	return nil
}
func required(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		slog.Error("required environment variable missing", "name", name)
		os.Exit(1)
	}
	return value
}
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
