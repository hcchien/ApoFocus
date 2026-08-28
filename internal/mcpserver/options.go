package mcpserver

import (
	"context"

	"github.com/hcchien/apofocus/internal/backup"
	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/maintenance"
	"github.com/hcchien/apofocus/internal/mediaingest"
)

type MediaImporter interface {
	Inspect(context.Context, mediaingest.ImportRequest) (mediaingest.Inspection, error)
	Import(context.Context, mediaingest.ImportRequest) (mediaingest.ImportResult, error)
}

type BatchJobs interface {
	Create(context.Context, batch.CreateInput) (batch.Job, error)
	List(context.Context, string, int) ([]batch.Job, error)
	Get(context.Context, string) (batch.Job, error)
	Items(context.Context, string, int) ([]batch.Item, error)
	Cancel(context.Context, string) error
	Resume(context.Context, string) (batch.Job, error)
}

type Options struct {
	PhotoImporter *ingest.Manager
	MediaImporter MediaImporter
	Photos        catalog.Store
	Media         catalog.MediaStore
	Folders       folders.Repository
	BatchJobs     BatchJobs
	Maintenance   maintenance.Checker
	Backup        backup.Operations
	ImportRoots   []string
	LibraryRoot   string
}
