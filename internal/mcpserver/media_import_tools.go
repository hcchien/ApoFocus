package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/mediaingest"
)

type InspectMediaInput struct {
	SourcePath string   `json:"source_path" jsonschema:"absolute local path for a video or audio file inside APOFOCUS_IMPORT_ROOTS"`
	Title      string   `json:"title,omitempty" jsonschema:"optional title; defaults to the filename"`
	Project    string   `json:"project,omitempty" jsonschema:"optional project or story name; defaults to 未分類"`
	Tags       []string `json:"tags,omitempty" jsonschema:"optional tags proposed by the photographer or LLM"`
	AutoTags   *bool    `json:"auto_tags,omitempty" jsonschema:"run local OpenCLIP and/or CLAP tag suggestions; defaults to true"`
}

type ImportMediaInput struct {
	SourcePath string   `json:"source_path" jsonschema:"absolute local path for a video or audio file inside APOFOCUS_IMPORT_ROOTS"`
	Title      string   `json:"title,omitempty" jsonschema:"title; defaults to the filename"`
	Project    string   `json:"project,omitempty" jsonschema:"project or story name; defaults to 未分類"`
	Tags       []string `json:"tags,omitempty" jsonschema:"tags selected by the photographer or LLM; merged with automatic tags"`
	AutoTags   *bool    `json:"auto_tags,omitempty" jsonschema:"add local OpenCLIP and/or CLAP tag suggestions; defaults to true"`
	Confirmed  bool     `json:"confirmed" jsonschema:"must be true only after the user reviews inspect_media"`
}

func addMediaImportTools(server *mcp.Server, importer MediaImporter) {
	closedWorld, additive := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_media", Title: "Inspect video or audio",
		Description: "Analyze an allowlisted local video or audio file without importing it. Returns codec and duration metadata, a transcript preview, suggested tags and folder, plus counts of OpenCLIP/CLAP vector segments. Analysis can take several minutes for long media.",
		Annotations: &mcp.ToolAnnotations{Title: "Inspect video or audio", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input InspectMediaInput) (*mcp.CallToolResult, mediaingest.Inspection, error) {
		result, err := importer.Inspect(ctx, mediaRequest(input.SourcePath, input.Title, input.Project, input.Tags, input.AutoTags))
		return nil, result, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "import_media", Title: "Import video or audio",
		Description: "After inspect_media is reviewed, copy a video or audio file into the managed library, generate a compact video thumbnail when applicable, transcript, temporary visual samples and OpenCLIP/CLAP vectors, merge tags, and atomically insert the asset and segments in PostgreSQL. Audio files do not create thumbnail artifacts. confirmed must be true; SHA-256 deduplication makes retries safe.",
		Annotations: &mcp.ToolAnnotations{Title: "Import video or audio", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ImportMediaInput) (*mcp.CallToolResult, mediaingest.ImportResult, error) {
		if !input.Confirmed {
			return nil, mediaingest.ImportResult{}, errors.New("confirmed must be true after the user reviews inspect_media")
		}
		result, err := importer.Import(ctx, mediaRequest(input.SourcePath, input.Title, input.Project, input.Tags, input.AutoTags))
		return nil, result, err
	})
}

func mediaRequest(sourcePath, title, project string, tags []string, autoTags *bool) mediaingest.ImportRequest {
	automatic := true
	if autoTags != nil {
		automatic = *autoTags
	}
	return mediaingest.ImportRequest{SourcePath: strings.TrimSpace(sourcePath), Title: title, Project: project, Tags: tags, AutoTags: automatic}
}
