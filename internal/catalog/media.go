package catalog

import (
	"context"
	"time"
)

type MediaAsset struct {
	ID             string         `json:"id"`
	MediaType      string         `json:"mediaType"`
	Title          string         `json:"title"`
	Year           int            `json:"year"`
	Project        string         `json:"project"`
	RecordedAt     time.Time      `json:"recordedAt"`
	DurationMS     int64          `json:"durationMs"`
	MimeType       string         `json:"mimeType"`
	Codec          string         `json:"codec"`
	Dimensions     string         `json:"dimensions,omitempty"`
	SampleRate     int            `json:"sampleRate,omitempty"`
	Channels       int            `json:"channels,omitempty"`
	Tags           []string       `json:"tags"`
	MediaURL       string         `json:"mediaUrl"`
	ThumbnailURL   string         `json:"thumbnailUrl"`
	Availability   string         `json:"availabilityStatus"`
	ThumbnailState string         `json:"thumbnailStatus"`
	Transcript     string         `json:"transcript,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Path           string         `json:"-"`
	ThumbnailPath  string         `json:"-"`
	Segments       []MediaSegment `json:"segments,omitempty"`
	Description    string         `json:"description"`
	Copyright      string         `json:"copyright"`
	Rating         int            `json:"rating"`
	Favorite       bool           `json:"favorite"`
	UserMetadata   map[string]any `json:"userMetadata,omitempty"`
	Revision       int64          `json:"revision"`
	HashStatus     string         `json:"hashStatus"`
	AIStatus       string         `json:"aiStatus"`
	DeepIndexState string         `json:"deepIndexStatus"`
}

type MediaUpdate struct {
	Title        *string         `json:"title,omitempty"`
	Project      *string         `json:"project,omitempty"`
	RecordedAt   *time.Time      `json:"recordedAt,omitempty"`
	Tags         *[]string       `json:"tags,omitempty"`
	Description  *string         `json:"description,omitempty"`
	Copyright    *string         `json:"copyright,omitempty"`
	Rating       *int            `json:"rating,omitempty"`
	Favorite     *bool           `json:"favorite,omitempty"`
	Transcript   *string         `json:"transcript,omitempty"`
	UserMetadata *map[string]any `json:"userMetadata,omitempty"`
	Revision     *int64          `json:"revision"`
}

type MediaSegment struct {
	ID            string         `json:"id"`
	SegmentType   string         `json:"segmentType"`
	Index         int            `json:"index"`
	StartMS       int64          `json:"startMs"`
	EndMS         int64          `json:"endMs"`
	KeyframeURL   string         `json:"keyframeUrl,omitempty"`
	KeyframeState string         `json:"keyframeStatus,omitempty"`
	Transcript    string         `json:"transcript,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type MediaFilter struct {
	MediaType     string
	Query         string
	Year          int
	Project       string
	Tags          []string
	Codec         string
	MinDuration   int64
	MaxDuration   int64
	HasTranscript *bool
	Limit         int
	Offset        int
}

type MediaPage struct {
	Items  []MediaAsset `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

type MediaFacets struct {
	Years    []FacetCount `json:"years"`
	Projects []FacetCount `json:"projects"`
	Tags     []FacetCount `json:"tags"`
	Codecs   []FacetCount `json:"codecs"`
}

type SimilarMedia struct {
	Asset      MediaAsset `json:"asset"`
	Similarity float64    `json:"similarity"`
}

type MediaStore interface {
	ListMedia(context.Context, MediaFilter) (MediaPage, error)
	GetMedia(context.Context, string, string) (MediaAsset, error)
	MediaFacets(context.Context, string) (MediaFacets, error)
	SimilarMedia(context.Context, string, string, string, int) ([]SimilarMedia, error)
	UpdateMedia(context.Context, string, string, MediaUpdate) (MediaAsset, error)
}
