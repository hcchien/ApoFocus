package storagewatch

import (
	"context"
	"errors"
	"time"

	"github.com/hcchien/apofocus/internal/fileidentity"
)

var ErrRootOffline = errors.New("storage root is offline")

type Root struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	BasePath   string     `json:"basePath"`
	VolumeID   string     `json:"volumeId"`
	Status     string     `json:"status"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	LastEvent  *time.Time `json:"lastEventAt,omitempty"`
}

type Tracker interface {
	ObservePath(context.Context, Root, string, fileidentity.Identity) error
	MarkMissing(context.Context, Root, string) error
	MarkRootOffline(context.Context, Root) error
	VerifyKnownPaths(context.Context, Root) error
	TouchRoot(context.Context, Root) error
}
