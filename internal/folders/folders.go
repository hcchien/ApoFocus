package folders

import (
	"context"
	"time"
)

type Collection struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parentId,omitempty"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Filter    map[string]any `json:"filter,omitempty"`
	Count     int            `json:"count"`
	CreatedAt time.Time      `json:"createdAt"`
}

type CreateInput struct {
	ParentID string         `json:"parentId,omitempty"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Filter   map[string]any `json:"filter,omitempty"`
}

type Repository interface {
	List(context.Context) ([]Collection, error)
	Create(context.Context, CreateInput) (Collection, error)
	AddPhotos(context.Context, string, []string) error
	PhotoIDs(context.Context, string) ([]string, error)
}
