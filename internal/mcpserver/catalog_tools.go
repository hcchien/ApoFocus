package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/catalog"
)

type SearchPhotosInput struct {
	Query       string   `json:"query,omitempty" jsonschema:"full-text query across title, project, tags, camera, lens, and indexed metadata"`
	Year        int      `json:"year,omitempty" jsonschema:"capture year"`
	Project     string   `json:"project,omitempty" jsonschema:"exact project name"`
	Tags        []string `json:"tags,omitempty" jsonschema:"all tags that must be present"`
	Camera      string   `json:"camera,omitempty" jsonschema:"exact camera model"`
	Lens        string   `json:"lens,omitempty" jsonschema:"exact lens model"`
	MinISO      int      `json:"min_iso,omitempty" jsonschema:"minimum ISO"`
	MaxISO      int      `json:"max_iso,omitempty" jsonschema:"maximum ISO"`
	HasLocation *bool    `json:"has_location,omitempty" jsonschema:"true for GeoTagged photos; false for photos without coordinates"`
	Limit       int      `json:"limit,omitempty" jsonschema:"results per page from 1 to 100; defaults to 30"`
	Offset      int      `json:"offset,omitempty" jsonschema:"zero-based pagination offset"`
}

type PhotoIDInput struct {
	PhotoID string `json:"photo_id" jsonschema:"ApoFocus photo UUID"`
}

type SimilarPhotosInput struct {
	PhotoID string `json:"photo_id" jsonschema:"anchor ApoFocus photo UUID"`
	Limit   int    `json:"limit,omitempty" jsonschema:"number of similar photos from 1 to 24; defaults to 6"`
}

type SearchMediaInput struct {
	MediaType     string   `json:"media_type" jsonschema:"video or audio"`
	Query         string   `json:"query,omitempty" jsonschema:"full-text query across title, transcript, project, and tags"`
	Year          int      `json:"year,omitempty" jsonschema:"recording year"`
	Project       string   `json:"project,omitempty" jsonschema:"exact project name"`
	Tags          []string `json:"tags,omitempty" jsonschema:"all tags that must be present"`
	Codec         string   `json:"codec,omitempty" jsonschema:"exact codec"`
	MinDuration   int64    `json:"min_duration_ms,omitempty" jsonschema:"minimum duration in milliseconds"`
	MaxDuration   int64    `json:"max_duration_ms,omitempty" jsonschema:"maximum duration in milliseconds"`
	HasTranscript *bool    `json:"has_transcript,omitempty" jsonschema:"true for media with a transcript; false for media without one"`
	Limit         int      `json:"limit,omitempty" jsonschema:"results per page from 1 to 100; defaults to 30"`
	Offset        int      `json:"offset,omitempty" jsonschema:"zero-based pagination offset"`
}

type MediaIDInput struct {
	MediaType string `json:"media_type" jsonschema:"video or audio"`
	AssetID   string `json:"asset_id" jsonschema:"ApoFocus media asset UUID"`
}

type SimilarMediaInput struct {
	MediaType string `json:"media_type" jsonschema:"video or audio"`
	AssetID   string `json:"asset_id" jsonschema:"anchor ApoFocus media asset UUID"`
	Modality  string `json:"modality" jsonschema:"visual or audio; visual is valid only for videos"`
	Limit     int    `json:"limit,omitempty" jsonschema:"number of similar assets from 1 to 24; defaults to 6"`
}

func addPhotoCatalogTools(server *mcp.Server, store catalog.Store) {
	readOnly, closedWorld := true, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "search_photos", Title: "Search photos",
		Description: "Search and filter the ApoFocus photo catalog by text, year, project, tags, EXIF fields, ISO range, and GeoTag availability. Local filesystem paths are never returned.",
		Annotations: &mcp.ToolAnnotations{Title: "Search photos", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SearchPhotosInput) (*mcp.CallToolResult, catalog.PhotoPage, error) {
		page, err := store.List(ctx, catalog.Filter{Query: strings.TrimSpace(input.Query), Year: input.Year, Project: strings.TrimSpace(input.Project), Tags: input.Tags, Camera: strings.TrimSpace(input.Camera), Lens: strings.TrimSpace(input.Lens), MinISO: input.MinISO, MaxISO: input.MaxISO, HasLocation: input.HasLocation, Limit: bounded(input.Limit, 30, 1, 100), Offset: max(input.Offset, 0)})
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_photo", Title: "Get photo details", Description: "Return one photo with EXIF, GeoTag, tags, URLs, metadata, and availability state. Local filesystem paths and embeddings are not returned.",
		Annotations: &mcp.ToolAnnotations{Title: "Get photo details", ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input PhotoIDInput) (*mcp.CallToolResult, catalog.Photo, error) {
		return photoResult(store.Get(ctx, strings.TrimSpace(input.PhotoID)))
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "find_similar_photos", Title: "Find similar photos", Description: "Use PostgreSQL pgvector cosine distance to find photos visually similar to an anchor photo.",
		Annotations: &mcp.ToolAnnotations{Title: "Find similar photos", ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SimilarPhotosInput) (*mcp.CallToolResult, []catalog.SimilarPhoto, error) {
		items, err := store.Similar(ctx, strings.TrimSpace(input.PhotoID), bounded(input.Limit, 6, 1, 24))
		return nil, items, err
	})
}

func addMediaCatalogTools(server *mcp.Server, store catalog.MediaStore) {
	closedWorld := false
	mcp.AddTool(server, &mcp.Tool{
		Name: "search_media", Title: "Search videos or audio", Description: "Search and filter videos or audio by text, year, project, tags, codec, duration, and transcript availability.",
		Annotations: &mcp.ToolAnnotations{Title: "Search videos or audio", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SearchMediaInput) (*mcp.CallToolResult, catalog.MediaPage, error) {
		mediaType, err := validMediaType(input.MediaType)
		if err != nil {
			return nil, catalog.MediaPage{}, err
		}
		page, err := store.ListMedia(ctx, catalog.MediaFilter{MediaType: mediaType, Query: strings.TrimSpace(input.Query), Year: input.Year, Project: strings.TrimSpace(input.Project), Tags: input.Tags, Codec: strings.TrimSpace(input.Codec), MinDuration: input.MinDuration, MaxDuration: input.MaxDuration, HasTranscript: input.HasTranscript, Limit: bounded(input.Limit, 30, 1, 100), Offset: max(input.Offset, 0)})
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_media", Title: "Get video or audio details", Description: "Return one video or audio asset with transcript, indexed segments, tags, metadata, URLs, and availability state. Local filesystem paths and vectors are not returned.",
		Annotations: &mcp.ToolAnnotations{Title: "Get video or audio details", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input MediaIDInput) (*mcp.CallToolResult, catalog.MediaAsset, error) {
		mediaType, err := validMediaType(input.MediaType)
		if err != nil {
			return nil, catalog.MediaAsset{}, err
		}
		asset, err := store.GetMedia(ctx, mediaType, strings.TrimSpace(input.AssetID))
		return nil, asset, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "find_similar_media", Title: "Find similar video or audio", Description: "Use pgvector cosine distance over OpenCLIP keyframes or CLAP audio segments to find similar media.",
		Annotations: &mcp.ToolAnnotations{Title: "Find similar video or audio", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SimilarMediaInput) (*mcp.CallToolResult, []catalog.SimilarMedia, error) {
		mediaType, err := validMediaType(input.MediaType)
		if err != nil {
			return nil, nil, err
		}
		modality := strings.ToLower(strings.TrimSpace(input.Modality))
		if modality != "visual" && modality != "audio" {
			return nil, nil, errors.New("modality must be visual or audio")
		}
		if mediaType == "audio" && modality == "visual" {
			return nil, nil, errors.New("visual similarity is available only for video")
		}
		items, err := store.SimilarMedia(ctx, mediaType, strings.TrimSpace(input.AssetID), modality, bounded(input.Limit, 6, 1, 24))
		return nil, items, err
	})
}

func validMediaType(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "video" && value != "audio" {
		return "", errors.New("media_type must be video or audio")
	}
	return value, nil
}

func bounded(value, fallback, minimum, maximum int) int {
	if value == 0 {
		return fallback
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func photoResult(photo catalog.Photo, err error) (*mcp.CallToolResult, catalog.Photo, error) {
	return nil, photo, err
}
