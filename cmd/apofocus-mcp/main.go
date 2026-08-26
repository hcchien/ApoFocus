package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/mcpserver"
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
	if err := db.PingContext(ctx); err != nil {
		cancel()
		logger.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	cancel()

	manager, err := ingest.NewManager(libraryRoot, importRoots, ingest.NewHTTPAnalyzer(embeddingURL), ingest.NewPostgresRepository(db))
	if err != nil {
		logger.Error("configure importer", "error", err)
		os.Exit(1)
	}
	server := mcpserver.New(manager, importRoots, libraryRoot)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
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
