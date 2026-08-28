package initjob

import (
	"context"
	"time"
)

type CreateInput struct {
	SourceRoot string   `json:"sourceRoot"`
	Project    string   `json:"project,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Recursive  bool     `json:"recursive"`
}

type Run struct {
	ID                string     `json:"id"`
	SourceRoot        string     `json:"sourceRoot"`
	Project           string     `json:"project"`
	Tags              []string   `json:"tags"`
	Recursive         bool       `json:"recursive"`
	Status            string     `json:"status"`
	DiscoveredCount   int        `json:"discoveredCount"`
	PhotoCount        int        `json:"photoCount"`
	MediaCount        int        `json:"mediaCount"`
	CatalogedCount    int        `json:"catalogedCount"`
	PhotoAICount      int        `json:"photoAiCount"`
	MediaAICount      int        `json:"mediaAiCount"`
	FailedCount       int        `json:"failedCount"`
	CurrentPath       string     `json:"currentPath,omitempty"`
	Error             string     `json:"error,omitempty"`
	PauseRequested    bool       `json:"pauseRequested"`
	CancelRequested   bool       `json:"cancelRequested"`
	DiscoveryComplete bool       `json:"discoveryComplete"`
	CreatedAt         time.Time  `json:"createdAt"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	HeartbeatAt       *time.Time `json:"heartbeatAt,omitempty"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
}

func (r Run) Terminal() bool {
	return r.Status == "completed" || r.Status == "completed_with_errors" || r.Status == "failed" || r.Status == "cancelled"
}

type Item struct {
	ID           int64      `json:"id"`
	RunID        string     `json:"runId"`
	SourcePath   string     `json:"sourcePath"`
	MediaType    string     `json:"mediaType"`
	SizeBytes    int64      `json:"sizeBytes"`
	ModifiedAt   *time.Time `json:"modifiedAt,omitempty"`
	FileID       string     `json:"fileId,omitempty"`
	Status       string     `json:"status"`
	AssetID      string     `json:"assetId,omitempty"`
	Error        string     `json:"error,omitempty"`
	AttemptCount int        `json:"attemptCount"`
}

type Discovered struct {
	Path       string
	MediaType  string
	SizeBytes  int64
	ModifiedAt time.Time
	FileID     string
}

type Repository interface {
	Create(context.Context, CreateInput) (Run, error)
	Get(context.Context, string) (Run, error)
	List(context.Context, string, int) ([]Run, error)
	Items(context.Context, string, int) ([]Item, error)
	RequestPause(context.Context, string) error
	Resume(context.Context, string) (Run, error)
	Cancel(context.Context, string) error
	ClaimRun(context.Context, string) (Run, bool, error)
	AddDiscovered(context.Context, string, []Discovered) error
	SetRunStage(context.Context, string, string) error
	ClaimItems(context.Context, string, string, string, int, string) ([]Item, error)
	CompleteCatalog(context.Context, string, Item, string, error) error
	CompleteAI(context.Context, string, Item, error) error
	Heartbeat(context.Context, string, string) (string, error)
	Finish(context.Context, string, error) error
}

type Processor interface {
	Catalog(context.Context, Run, Item) (string, error)
	AnalyzePhoto(context.Context, Run, Item) error
	AnalyzeMedia(context.Context, Run, Item) error
}
type PhotoBatchProcessor interface {
	AnalyzePhotoBatch(context.Context, Run, []Item) map[int64]error
}

// PathExcluder lets a processor keep its own managed output out of discovery.
// This is especially important when a user intentionally scans /Volumes.
type PathExcluder interface {
	ExcludePath(string) bool
}
