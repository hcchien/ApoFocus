package batch

import (
	"context"
	"time"

	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/mediaingest"
)

type CreateInput struct {
	SourceRoot string   `json:"sourceRoot"`
	Project    string   `json:"project,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Recursive  bool     `json:"recursive"`
	AutoTags   bool     `json:"autoTags"`
	MediaTypes []string `json:"mediaTypes,omitempty"`
}

type Job struct {
	ID              string     `json:"id"`
	SourceRoot      string     `json:"sourceRoot"`
	Project         string     `json:"project"`
	Tags            []string   `json:"tags"`
	Recursive       bool       `json:"recursive"`
	AutoTags        bool       `json:"autoTags"`
	MediaTypes      []string   `json:"mediaTypes"`
	Status          string     `json:"status"`
	DiscoveredCount int        `json:"discoveredCount"`
	ProcessedCount  int        `json:"processedCount"`
	SucceededCount  int        `json:"succeededCount"`
	FailedCount     int        `json:"failedCount"`
	CurrentPath     string     `json:"currentPath,omitempty"`
	Error           string     `json:"error,omitempty"`
	CancelRequested bool       `json:"cancelRequested"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

func (j Job) Terminal() bool {
	return j.Status == "completed" || j.Status == "completed_with_errors" || j.Status == "failed" || j.Status == "cancelled"
}

type Item struct {
	ID           int64      `json:"id"`
	SourcePath   string     `json:"sourcePath"`
	Status       string     `json:"status"`
	PhotoID      string     `json:"photoId,omitempty"`
	MediaType    string     `json:"mediaType"`
	MediaAssetID string     `json:"mediaAssetId,omitempty"`
	Error        string     `json:"error,omitempty"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

type Repository interface {
	Create(context.Context, CreateInput) (Job, error)
	Get(context.Context, string) (Job, error)
	Items(context.Context, string, int) ([]Item, error)
	Cancel(context.Context, string) error
	ClaimNext(context.Context) (Job, bool, error)
	AddDiscovered(context.Context, string, []DiscoveredFile) error
	StartRunning(context.Context, string) error
	NextItem(context.Context, string) (Item, bool, error)
	CompleteItem(context.Context, string, int64, string, string, error) error
	Finish(context.Context, string, error) error
	Heartbeat(context.Context, string, string) (bool, error)
}

type Importer interface {
	ValidateBatchRoot(string) (string, error)
	Import(context.Context, ingest.ImportRequest) (ingest.ImportResult, error)
}

type MediaImporter interface {
	Import(context.Context, mediaingest.ImportRequest) (mediaingest.ImportResult, error)
}

type DiscoveredFile struct {
	Path      string
	MediaType string
}
