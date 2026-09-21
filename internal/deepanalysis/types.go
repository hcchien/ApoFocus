package deepanalysis

import (
	"context"
	"encoding/json"
	"time"
)

const (
	DefaultModel         = "Qwen/Qwen2-VL-7B-Instruct"
	DefaultPromptVersion = "apofocus-photo-v1"
)

var ErrNotFound = errNotFound("deep analysis job not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

type CreateInput struct {
	PhotoIDs []string `json:"photoIds"`
	Force    bool     `json:"force,omitempty"`
}

type Job struct {
	ID              string     `json:"id"`
	Model           string     `json:"model"`
	PromptVersion   string     `json:"promptVersion"`
	Status          string     `json:"status"`
	SelectedCount   int        `json:"selectedCount"`
	ProcessedCount  int        `json:"processedCount"`
	SucceededCount  int        `json:"succeededCount"`
	FailedCount     int        `json:"failedCount"`
	Error           string     `json:"error,omitempty"`
	CancelRequested bool       `json:"cancelRequested"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	HeartbeatAt     *time.Time `json:"heartbeatAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

func (j Job) Terminal() bool {
	return j.Status == "completed" || j.Status == "completed_with_errors" || j.Status == "failed" || j.Status == "cancelled"
}

type Item struct {
	ID             int64           `json:"id"`
	JobID          string          `json:"jobId"`
	PhotoID        string          `json:"photoId"`
	SourceRevision int64           `json:"sourceRevision"`
	Status         string          `json:"status"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	AttemptCount   int             `json:"attemptCount"`
	StartedAt      *time.Time      `json:"startedAt,omitempty"`
	FinishedAt     *time.Time      `json:"finishedAt,omitempty"`
	Path           string          `json:"-"`
}

type PhotoAnalysis struct {
	PhotoID       string          `json:"photoId"`
	Status        string          `json:"status"`
	Model         string          `json:"model,omitempty"`
	PromptVersion string          `json:"promptVersion,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	AnalyzedAt    *time.Time      `json:"analyzedAt,omitempty"`
}

type Analyzer interface {
	Analyze(context.Context, string) (json.RawMessage, error)
}

type Repository interface {
	Create(context.Context, CreateInput, string, string) (Job, error)
	Get(context.Context, string) (Job, error)
	Items(context.Context, string, int) ([]Item, error)
	LatestForPhoto(context.Context, string) (PhotoAnalysis, error)
	Cancel(context.Context, string) error
	ClaimNext(context.Context) (Job, bool, error)
	NextItem(context.Context, string) (Item, bool, error)
	CompleteItem(context.Context, Job, Item, json.RawMessage, error) error
	Heartbeat(context.Context, string) (bool, error)
	Finish(context.Context, string, error) error
}
