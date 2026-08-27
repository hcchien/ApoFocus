package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/maintenance"
	"github.com/hcchien/apofocus/internal/mcpserver"
	"github.com/hcchien/apofocus/internal/mediaingest"
	"github.com/hcchien/apofocus/internal/storagewatch"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	databaseURL := required(logger, "DATABASE_URL")
	libraryRoot := required(logger, "PHOTO_LIBRARY_ROOT")
	importRoots := splitPaths(required(logger, "APOFOCUS_IMPORT_ROOTS"))
	embeddingURL := envOr("EMBEDDING_SERVICE_URL", "http://127.0.0.1:8090")

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		logger.Error("open PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pingErr := db.PingContext(ctx)
	cancel()
	if pingErr != nil {
		logger.Warn("PostgreSQL is offline; MCP is starting in maintenance mode", "error", pingErr)
		maintenanceManager := maintenance.NewManager(db, nil, libraryRoot, importRoots, envOr("APOFOCUS_APP_URL", "http://127.0.0.1:8080"), embeddingURL)
		serverContext, stop := context.WithCancel(context.Background())
		defer stop()
		server := mcpserver.NewWithOptions(mcpserver.Options{
			Maintenance: maintenanceManager, ImportRoots: importRoots, LibraryRoot: libraryRoot,
		})
		if err := server.Run(serverContext, &mcp.StdioTransport{}); err != nil {
			logger.Error("run maintenance MCP server", "error", err)
			os.Exit(1)
		}
		return
	}

	storageRepository := storagewatch.NewPostgresRepository(db)
	root, err := storageRepository.EnsureRoot(context.Background(), libraryRoot)
	libraryOnline := err == nil
	if errors.Is(err, storagewatch.ErrRootOffline) {
		logger.Warn("managed library is offline; MCP is starting in maintenance mode", "path", libraryRoot)
	} else if err != nil {
		logger.Error("configure managed library", "error", err)
		os.Exit(1)
	}
	photoStore := catalog.NewPostgresStore(db)
	folderRepository := folders.NewPostgresRepository(db)
	batchRepository := batch.NewPostgresRepository(db)
	var manager *ingest.Manager
	var mediaManager *mediaingest.Manager
	if libraryOnline {
		manager, err = ingest.NewManager(root.BasePath, importRoots, ingest.NewHTTPAnalyzer(embeddingURL), ingest.NewPostgresRepository(db, root.ID))
		if err != nil {
			logger.Error("configure importer", "error", err)
			os.Exit(1)
		}
		mediaManager, err = mediaingest.NewManager(root.BasePath, importRoots, mediaingest.NewHTTPAnalyzer(embeddingURL), mediaingest.NewPostgresRepository(db, root.ID))
		if err != nil {
			logger.Error("configure media importer", "error", err)
			os.Exit(1)
		}
	}
	batchJobs := batch.NewService(batchRepository, manager)
	maintenanceManager := maintenance.NewManager(db, batchJobs, libraryRoot, importRoots, envOr("APOFOCUS_APP_URL", "http://127.0.0.1:8080"), embeddingURL)
	serverContext, stop := context.WithCancel(context.Background())
	defer stop()
	if libraryOnline {
		watcher, watcherErr := storagewatch.NewWatcher(root, storageRepository, logger)
		if watcherErr != nil {
			logger.Error("watch managed library", "error", watcherErr)
			os.Exit(1)
		}
		go func() {
			if watchErr := watcher.Run(serverContext); watchErr != nil && !errors.Is(watchErr, context.Canceled) {
				logger.Error("filesystem watcher stopped", "error", watchErr)
			}
		}()
	}
	server := mcpserver.NewWithOptions(mcpserver.Options{
		PhotoImporter: manager, MediaImporter: mediaManager, Photos: photoStore, Media: photoStore,
		Folders: folderRepository, BatchJobs: batchJobs, Maintenance: maintenanceManager,
		ImportRoots: importRoots, LibraryRoot: libraryRoot,
	})
	if err := server.Run(serverContext, &mcp.StdioTransport{}); err != nil {
		logger.Error("run MCP server", "error", err)
		os.Exit(1)
	}
}

func required(logger *slog.Logger, name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		logger.Error("required environment variable is not set", "name", name)
		os.Exit(1)
	}
	return value
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
