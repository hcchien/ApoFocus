package mcpserver

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/ingest"
)

type InspectPhotoInput struct {
	SourcePath   string   `json:"source_path" jsonschema:"absolute local path supplied by the MCP host for the attached photo"`
	Title        string   `json:"title,omitempty" jsonschema:"optional photographer-facing title; defaults to the filename"`
	Project      string   `json:"project,omitempty" jsonschema:"optional project or story name; defaults to 未分類"`
	LocationName string   `json:"location_name,omitempty" jsonschema:"optional human-readable location corresponding to the EXIF coordinates"`
	Tags         []string `json:"tags,omitempty" jsonschema:"optional tags proposed by the photographer or LLM"`
	AutoTags     *bool    `json:"auto_tags,omitempty" jsonschema:"run local OpenCLIP tag suggestions; defaults to true"`
}

type ImportPhotoInput struct {
	SourcePath   string   `json:"source_path" jsonschema:"absolute local path supplied by the MCP host for the attached photo"`
	Title        string   `json:"title,omitempty" jsonschema:"photographer-facing title; defaults to the filename"`
	Project      string   `json:"project,omitempty" jsonschema:"project or story name; defaults to 未分類"`
	LocationName string   `json:"location_name,omitempty" jsonschema:"human-readable location corresponding to EXIF coordinates"`
	Tags         []string `json:"tags,omitempty" jsonschema:"tags selected by the photographer or LLM; merged with automatic tags"`
	AutoTags     *bool    `json:"auto_tags,omitempty" jsonschema:"add local OpenCLIP tag suggestions; defaults to true"`
	Confirmed    bool     `json:"confirmed" jsonschema:"must be true only after the user has reviewed the proposed import"`
}

type PolicyInput struct {
	Locale string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for human-readable policy notes"`
}

type PolicyOutput struct {
	ImportRoots   []string `json:"importRoots"`
	LibraryRoot   string   `json:"libraryRoot"`
	FolderPattern string   `json:"folderPattern"`
	SourceAction  string   `json:"sourceAction"`
	Notes         []string `json:"notes"`
}

func New(manager *ingest.Manager, importRoots []string, libraryRoot string) *mcp.Server {
	return NewWithOptions(Options{PhotoImporter: manager, ImportRoots: importRoots, LibraryRoot: libraryRoot})
}

func NewWithOptions(options Options) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "apofocus", Version: "v0.7.0"}, nil)
	addPhotoImportTools(server, options.PhotoImporter, options.ImportRoots, options.LibraryRoot)
	if options.Photos != nil {
		addPhotoCatalogTools(server, options.Photos)
	}
	if options.Media != nil {
		addMediaCatalogTools(server, options.Media)
	}
	if options.Relations != nil {
		addRelationTools(server, options.Relations)
	}
	if options.MediaImporter != nil {
		addMediaImportTools(server, options.MediaImporter)
	}
	if options.Folders != nil && options.Photos != nil {
		addFolderTools(server, options.Folders, options.Photos, options.Media)
	}
	if options.BatchJobs != nil {
		addBatchTools(server, options.BatchJobs)
	}
	if options.InitJobs != nil {
		addInitTools(server, options.InitJobs)
	}
	if options.Maintenance != nil {
		addMaintenanceTools(server, options.Maintenance, options.BatchJobs)
	}
	if options.Backup != nil {
		addBackupTools(server, options.Backup)
	}
	return server
}

func addPhotoImportTools(server *mcp.Server, manager *ingest.Manager, importRoots []string, libraryRoot string) {
	if manager == nil {
		return
	}
	closedWorld := false
	additive := false

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_photo_import_policy",
		Title:       "取得照片匯入規則",
		Description: "Return the local allowlisted import roots and deterministic folder policy before choosing a source photo.",
		Annotations: &mcp.ToolAnnotations{Title: "取得照片匯入規則", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input PolicyInput) (*mcp.CallToolResult, PolicyOutput, error) {
		locale := normalizedLocale(input.Locale)
		return nil, PolicyOutput{
			ImportRoots: importRoots, LibraryRoot: libraryRoot,
			FolderPattern: filepath.Join(libraryRoot, "originals", "YYYY", "project", "YYYY-MM-DD_filename_sha.ext"),
			SourceAction:  localized(locale, "複製；來源附件會保留", "copy; the source attachment is preserved", "Kopie; die Quelldatei bleibt erhalten"),
			Notes: []string{
				localized(locale, "在 import_photo 前先呼叫 inspect_photo。", "Call inspect_photo before import_photo.", "Vor import_photo zuerst inspect_photo aufrufen."),
				localized(locale, "只接受 import root 內的路徑。", "Only paths inside an import root are accepted.", "Nur Pfade innerhalb eines Import-Roots werden akzeptiert."),
				localized(locale, "匯入會用 SHA-256 去重。", "Imports are deduplicated by SHA-256.", "Importe werden anhand von SHA-256 dedupliziert."),
			},
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_photo", Title: "檢查照片與建議分類",
		Description: "Inspect an attached local photo, read EXIF and GeoTag, calculate its deterministic destination folder, and optionally suggest tags with local OpenCLIP. This tool does not copy files or write the database. The source_path must be inside APOFOCUS_IMPORT_ROOTS.",
		Annotations: &mcp.ToolAnnotations{Title: "檢查照片與建議分類", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input InspectPhotoInput) (*mcp.CallToolResult, ingest.Inspection, error) {
		result, err := manager.Inspect(ctx, toImportRequest(input.SourcePath, input.Title, input.Project, input.LocationName, input.Tags, input.AutoTags))
		return nil, result, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "import_photo", Title: "匯入照片至 ApoFocus",
		Description: "After the user reviews inspect_photo, copy the photo into the managed local library, create a thumbnail, calculate a 512-dimensional OpenCLIP vector, merge automatic and supplied tags, and atomically add the photo, project, and tags to PostgreSQL. confirmed must be true. Repeating an import is safe because content is deduplicated by SHA-256.",
		Annotations: &mcp.ToolAnnotations{Title: "匯入照片至 ApoFocus", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ImportPhotoInput) (*mcp.CallToolResult, ingest.ImportResult, error) {
		if !input.Confirmed {
			return nil, ingest.ImportResult{}, errors.New("confirmed must be true after the user reviews inspect_photo")
		}
		result, err := manager.Import(ctx, toImportRequest(input.SourcePath, input.Title, input.Project, input.LocationName, input.Tags, input.AutoTags))
		return nil, result, err
	})
}

func toImportRequest(sourcePath, title, project, locationName string, tags []string, autoTags *bool) ingest.ImportRequest {
	automatic := true
	if autoTags != nil {
		automatic = *autoTags
	}
	return ingest.ImportRequest{
		SourcePath: sourcePath, Title: title, Project: project, LocationName: locationName,
		Tags: tags, AutoTags: automatic,
	}
}
