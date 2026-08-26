package mediaingest

import (
	"context"
	"time"
)

type ImportRequest struct {
	SourcePath string
	Title      string
	Project    string
	Tags       []string
	AutoTags   bool
}

type Segment struct {
	SegmentType          string         `json:"segmentType"`
	Index                int            `json:"index"`
	StartMS              int64          `json:"startMs"`
	EndMS                int64          `json:"endMs"`
	KeyframePath         string         `json:"keyframePath"`
	KeyframeRelativePath string         `json:"-"`
	KeyframeFileID       string         `json:"-"`
	KeyframeURL          string         `json:"-"`
	Transcript           string         `json:"transcript"`
	Tags                 []string       `json:"tags"`
	VisualVector         []float32      `json:"visualVector"`
	AudioVector          []float32      `json:"audioVector"`
	Metadata             map[string]any `json:"metadata,omitempty"`
}

type Analysis struct {
	MediaType  string         `json:"mediaType"`
	DurationMS int64          `json:"durationMs"`
	MimeType   string         `json:"mimeType"`
	Codec      string         `json:"codec"`
	Dimensions string         `json:"dimensions"`
	SampleRate int            `json:"sampleRate"`
	Channels   int            `json:"channels"`
	RecordedAt string         `json:"recordedAt"`
	Transcript string         `json:"transcript"`
	Tags       []string       `json:"tags"`
	Metadata   map[string]any `json:"metadata"`
	Segments   []Segment      `json:"segments"`
}

type Analyzer interface {
	AnalyzeMedia(context.Context, string, string, string, bool) (Analysis, error)
}

type ExistingMedia struct {
	ID            string
	MediaType     string
	Path          string
	ThumbnailPath string
	Tags          []string
}

type Record struct {
	MediaType             string
	Title                 string
	Year                  int
	Project               string
	RecordedAt            time.Time
	DurationMS            int64
	MimeType              string
	Codec                 string
	Dimensions            string
	SampleRate            int
	Channels              int
	Path                  string
	ThumbnailPath         string
	RelativePath          string
	ThumbnailRelativePath string
	FileID                string
	ThumbnailFileID       string
	ContentSHA256         string
	MediaURL              string
	ThumbnailURL          string
	Transcript            string
	Tags                  []string
	Metadata              map[string]any
	Segments              []Segment
}

type Repository interface {
	FindByHash(context.Context, string) (ExistingMedia, bool, error)
	Insert(context.Context, Record) (string, error)
}

type ImportResult struct {
	AssetID          string   `json:"assetId"`
	MediaType        string   `json:"mediaType"`
	Path             string   `json:"path"`
	ThumbnailPath    string   `json:"thumbnailPath"`
	Tags             []string `json:"tags"`
	AlreadyExists    bool     `json:"alreadyExists"`
	SegmentCount     int      `json:"segmentCount"`
	TranscriptLength int      `json:"transcriptLength"`
}
