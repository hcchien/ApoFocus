package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/ingest"
)

type noOpAnalyzer struct{}

func (noOpAnalyzer) Analyze(context.Context, string, string) (ingest.Analysis, error) {
	return ingest.Analysis{Embedding: make([]float32, 512)}, nil
}

func TestServerAdvertisesAndCallsTools(t *testing.T) {
	ctx := context.Background()
	inbox, library := t.TempDir(), t.TempDir()
	manager, err := ingest.NewManager(library, []string{inbox}, noOpAnalyzer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(manager, []string{inbox}, library)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools.Tools))
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "get_photo_import_policy", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("policy tool returned an error: %+v", result.Content)
	}
	if result.StructuredContent == nil {
		t.Fatal("policy tool did not return structured content")
	}
}
