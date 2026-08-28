package ingest

import (
	"context"
	"time"
)

type Location struct {
	Name      string  `json:"name,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ImportRequest struct {
	SourcePath   string
	Title        string
	Project      string
	LocationName string
	Tags         []string
	AutoTags     bool
}

type Inspection struct {
	SourcePath      string             `json:"sourcePath"`
	Filename        string             `json:"filename"`
	ContentSHA256   string             `json:"contentSha256"`
	Title           string             `json:"title"`
	Project         string             `json:"project"`
	TakenAt         time.Time          `json:"takenAt"`
	Year            int                `json:"year"`
	Camera          string             `json:"camera,omitempty"`
	Lens            string             `json:"lens,omitempty"`
	Aperture        string             `json:"aperture,omitempty"`
	ShutterSpeed    string             `json:"shutterSpeed,omitempty"`
	ISO             int                `json:"iso,omitempty"`
	FocalLength     string             `json:"focalLength,omitempty"`
	Dimensions      string             `json:"dimensions,omitempty"`
	FileType        string             `json:"fileType"`
	FileSizeBytes   int64              `json:"fileSizeBytes"`
	Location        *Location          `json:"location,omitempty"`
	SuggestedFolder string             `json:"suggestedFolder"`
	SuggestedTags   []string           `json:"suggestedTags"`
	Metadata        map[string]any     `json:"metadata"`
	AspectRatio     string             `json:"aspectRatio"`
	DominantColor   string             `json:"dominantColor,omitempty"`
	AnalysisTimings map[string]float64 `json:"analysisTimingsMs,omitempty"`
	embedding       []float32
}

type ImportResult struct {
	PhotoID          string             `json:"photoId"`
	Path             string             `json:"path"`
	ThumbnailPath    string             `json:"thumbnailPath"`
	Tags             []string           `json:"tags"`
	AlreadyExists    bool               `json:"alreadyExists"`
	VectorDimensions int                `json:"vectorDimensions"`
	AnalysisTimings  map[string]float64 `json:"analysisTimingsMs,omitempty"`
}

type Analysis struct {
	Tags          []string
	Embedding     []float32
	DominantColor string
	TimingsMS     map[string]float64
}

type Analyzer interface {
	Analyze(context.Context, string, string) (Analysis, error)
}

type ExistingPhoto struct {
	ID            string
	Path          string
	ThumbnailPath string
	Tags          []string
}

type PhotoRecord struct {
	Inspection
	Path                  string
	ThumbnailPath         string
	RelativePath          string
	ThumbnailRelativePath string
	FileID                string
	ThumbnailFileID       string
	ImageURL              string
	ThumbnailURL          string
	Tags                  []string
}

type Repository interface {
	FindByHash(context.Context, string) (ExistingPhoto, bool, error)
	Insert(context.Context, PhotoRecord) (string, error)
}
