package backup

import (
	"context"
	"time"
)

const (
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusUnhealthy = "unhealthy"
)

type State struct {
	Version            int        `json:"version"`
	RunningOperation   string     `json:"runningOperation,omitempty"`
	OperationStartedAt *time.Time `json:"operationStartedAt,omitempty"`
	LastBackupAt       *time.Time `json:"lastBackupAt,omitempty"`
	LastBackupPath     string     `json:"lastBackupPath,omitempty"`
	LastBackupBytes    int64      `json:"lastBackupBytes,omitempty"`
	LastVerifiedAt     *time.Time `json:"lastVerifiedAt,omitempty"`
	LastVerifiedPath   string     `json:"lastVerifiedPath,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
	LastErrorAt        *time.Time `json:"lastErrorAt,omitempty"`
}

type HealthReport struct {
	Status             string     `json:"status"`
	CheckedAt          time.Time  `json:"checkedAt"`
	Configured         bool       `json:"configured"`
	Root               string     `json:"root,omitempty"`
	RootAvailable      bool       `json:"rootAvailable"`
	ExpectedVolumeUUID string     `json:"expectedVolumeUuid,omitempty"`
	ActualVolumeUUID   string     `json:"actualVolumeUuid,omitempty"`
	FreeBytes          uint64     `json:"freeBytes,omitempty"`
	TotalBytes         uint64     `json:"totalBytes,omitempty"`
	BackupCount        int        `json:"backupCount"`
	BackupBytes        int64      `json:"backupBytes"`
	RunningOperation   string     `json:"runningOperation,omitempty"`
	OperationStartedAt *time.Time `json:"operationStartedAt,omitempty"`
	LastBackupAt       *time.Time `json:"lastBackupAt,omitempty"`
	LastBackupPath     string     `json:"lastBackupPath,omitempty"`
	LastBackupBytes    int64      `json:"lastBackupBytes,omitempty"`
	LastVerifiedAt     *time.Time `json:"lastVerifiedAt,omitempty"`
	LastVerifiedPath   string     `json:"lastVerifiedPath,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
	Detail             string     `json:"detail,omitempty"`
}

type TriggerResult struct {
	Accepted bool      `json:"accepted"`
	Service  string    `json:"service"`
	Action   string    `json:"action"`
	At       time.Time `json:"at"`
}

type Operations interface {
	Health(context.Context) (HealthReport, error)
	TriggerBackup(context.Context) (TriggerResult, error)
	TriggerVerify(context.Context) (TriggerResult, error)
}

type TriggerController interface {
	Kickstart(context.Context, string) (string, error)
}
