package maintenance

import (
	"context"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
)

const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
	StatusUnknown   = "unknown"
)

type ComponentHealth struct {
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	LatencyMS int64  `json:"latencyMs,omitempty"`
}

type PathHealth struct {
	Role        string  `json:"role"`
	Path        string  `json:"path"`
	Status      string  `json:"status"`
	Detail      string  `json:"detail,omitempty"`
	FreeBytes   uint64  `json:"freeBytes,omitempty"`
	TotalBytes  uint64  `json:"totalBytes,omitempty"`
	FreePercent float64 `json:"freePercent,omitempty"`
}

type WorkerHealth struct {
	Status          string     `json:"status"`
	Detail          string     `json:"detail,omitempty"`
	ActiveJobs      int        `json:"activeJobs"`
	PendingJobs     int        `json:"pendingJobs"`
	StaleJobs       int        `json:"staleJobs"`
	LatestHeartbeat *time.Time `json:"latestHeartbeat,omitempty"`
}

type HealthReport struct {
	Status     string          `json:"status"`
	CheckedAt  time.Time       `json:"checkedAt"`
	Database   ComponentHealth `json:"database"`
	Web        ComponentHealth `json:"web"`
	Embedding  ComponentHealth `json:"embedding"`
	Worker     WorkerHealth    `json:"worker"`
	Paths      []PathHealth    `json:"paths"`
	Repairable []string        `json:"repairableServices"`
}

type RepairResult struct {
	Service   string          `json:"service"`
	Label     string          `json:"label"`
	Action    string          `json:"action"`
	Before    ComponentHealth `json:"before"`
	After     ComponentHealth `json:"after"`
	Succeeded bool            `json:"succeeded"`
}

type Pinger interface {
	PingContext(context.Context) error
}

type BatchLister interface {
	List(context.Context, string, int) ([]batch.Job, error)
}

type Controller interface {
	Restart(context.Context, string) (string, error)
}

type Checker interface {
	Check(context.Context) (HealthReport, error)
	Repair(context.Context, string) (RepairResult, error)
}
