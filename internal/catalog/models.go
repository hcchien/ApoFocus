package catalog

import (
	"context"
	"time"
)

type Location struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Photo struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Year           int            `json:"year"`
	Project        string         `json:"project"`
	TakenAt        time.Time      `json:"takenAt"`
	Tags           []string       `json:"tags"`
	Camera         string         `json:"camera"`
	Lens           string         `json:"lens"`
	Aperture       string         `json:"aperture"`
	ShutterSpeed   string         `json:"shutterSpeed"`
	ISO            int            `json:"iso"`
	FocalLength    string         `json:"focalLength"`
	Dimensions     string         `json:"dimensions"`
	FileType       string         `json:"fileType"`
	FileSize       string         `json:"fileSize"`
	Location       *Location      `json:"location,omitempty"`
	Path           string         `json:"-"`
	ThumbnailPath  string         `json:"-"`
	ImageURL       string         `json:"imageUrl"`
	ThumbnailURL   string         `json:"thumbnailUrl"`
	AspectRatio    string         `json:"aspectRatio"`
	Dominant       string         `json:"dominantColor"`
	Availability   string         `json:"availabilityStatus"`
	ThumbnailState string         `json:"thumbnailStatus"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Description    string         `json:"description"`
	Copyright      string         `json:"copyright"`
	Rating         int            `json:"rating"`
	Favorite       bool           `json:"favorite"`
	UserMetadata   map[string]any `json:"userMetadata,omitempty"`
	Revision       int64          `json:"revision"`
	HashStatus     string         `json:"hashStatus"`
	AIStatus       string         `json:"aiStatus"`
	Embedding      []float32      `json:"-"`
}

type PhotoUpdate struct {
	Title         *string         `json:"title,omitempty"`
	Project       *string         `json:"project,omitempty"`
	TakenAt       *time.Time      `json:"takenAt,omitempty"`
	Tags          *[]string       `json:"tags,omitempty"`
	Camera        *string         `json:"camera,omitempty"`
	Lens          *string         `json:"lens,omitempty"`
	Aperture      *string         `json:"aperture,omitempty"`
	ShutterSpeed  *string         `json:"shutterSpeed,omitempty"`
	ISO           *int            `json:"iso,omitempty"`
	FocalLength   *string         `json:"focalLength,omitempty"`
	Location      *Location       `json:"location,omitempty"`
	ClearLocation bool            `json:"clearLocation,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Copyright     *string         `json:"copyright,omitempty"`
	Rating        *int            `json:"rating,omitempty"`
	Favorite      *bool           `json:"favorite,omitempty"`
	UserMetadata  *map[string]any `json:"userMetadata,omitempty"`
	Revision      *int64          `json:"revision"`
}

type Filter struct {
	Query       string
	Year        int
	Project     string
	Tags        []string
	Camera      string
	Lens        string
	MinISO      int
	MaxISO      int
	HasLocation *bool
	Limit       int
	Offset      int
}

type PhotoPage struct {
	Items  []Photo `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type FacetCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type Facets struct {
	Years    []FacetCount `json:"years"`
	Projects []FacetCount `json:"projects"`
	Tags     []FacetCount `json:"tags"`
	Cameras  []FacetCount `json:"cameras"`
	Lenses   []FacetCount `json:"lenses"`
}

type SimilarPhoto struct {
	Photo      Photo   `json:"photo"`
	Similarity float64 `json:"similarity"`
}

type Store interface {
	List(context.Context, Filter) (PhotoPage, error)
	Get(context.Context, string) (Photo, error)
	Facets(context.Context) (Facets, error)
	Similar(context.Context, string, int) ([]SimilarPhoto, error)
	Update(context.Context, string, PhotoUpdate) (Photo, error)
}
