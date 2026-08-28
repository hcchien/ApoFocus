package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

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

type UpdatePhotoInput struct {
	PhotoID       string            `json:"photo_id" jsonschema:"ApoFocus photo UUID"`
	Title         *string           `json:"title,omitempty" jsonschema:"photographer-authored title"`
	Project       *string           `json:"project,omitempty" jsonschema:"project or story; an empty value clears it"`
	TakenAt       *string           `json:"taken_at,omitempty" jsonschema:"RFC3339 capture date and time"`
	Tags          *[]string         `json:"tags,omitempty" jsonschema:"complete photographer-authored tag list"`
	Camera        *string           `json:"camera,omitempty" jsonschema:"corrected camera model"`
	Lens          *string           `json:"lens,omitempty" jsonschema:"corrected lens model"`
	Aperture      *string           `json:"aperture,omitempty"`
	ShutterSpeed  *string           `json:"shutter_speed,omitempty"`
	ISO           *int              `json:"iso,omitempty"`
	FocalLength   *string           `json:"focal_length,omitempty"`
	Location      *catalog.Location `json:"location,omitempty" jsonschema:"corrected location and coordinates"`
	ClearLocation bool              `json:"clear_location,omitempty"`
	Description   *string           `json:"description,omitempty"`
	Copyright     *string           `json:"copyright,omitempty"`
	Rating        *int              `json:"rating,omitempty" jsonschema:"rating from 0 to 5"`
	Favorite      *bool             `json:"favorite,omitempty"`
	UserMetadata  *map[string]any   `json:"user_metadata,omitempty"`
	Revision      *int64            `json:"revision" jsonschema:"required revision returned by get_photo; prevents overwriting concurrent edits"`
	Confirmed     bool              `json:"confirmed" jsonschema:"must be true after the user approves this catalog edit"`
}

type UpdateMediaInput struct {
	MediaType    string          `json:"media_type" jsonschema:"video or audio"`
	AssetID      string          `json:"asset_id" jsonschema:"ApoFocus media asset UUID"`
	Title        *string         `json:"title,omitempty"`
	Project      *string         `json:"project,omitempty"`
	RecordedAt   *string         `json:"recorded_at,omitempty" jsonschema:"RFC3339 recording date and time"`
	Tags         *[]string       `json:"tags,omitempty"`
	Description  *string         `json:"description,omitempty"`
	Copyright    *string         `json:"copyright,omitempty"`
	Rating       *int            `json:"rating,omitempty" jsonschema:"rating from 0 to 5"`
	Favorite     *bool           `json:"favorite,omitempty"`
	Transcript   *string         `json:"transcript,omitempty" jsonschema:"photographer-corrected transcript"`
	UserMetadata *map[string]any `json:"user_metadata,omitempty"`
	Revision     *int64          `json:"revision" jsonschema:"required revision returned by get_media"`
	Confirmed    bool            `json:"confirmed"`
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
	additive := false
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_photo_metadata", Title: "Update photo metadata",
		Description: "Update photographer-controlled fields without changing the file path, hash, vector, or system status. User tags replace the visible tag list and take precedence over later AI work.",
		Annotations: &mcp.ToolAnnotations{Title: "Update photo metadata", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdatePhotoInput) (*mcp.CallToolResult, catalog.Photo, error) {
		if !input.Confirmed {
			return nil, catalog.Photo{}, errors.New("confirmed must be true after the user approves the edit")
		}
		if input.Revision == nil {
			return nil, catalog.Photo{}, errors.New("revision is required; call get_photo before editing")
		}
		var takenAt *time.Time
		if input.TakenAt != nil {
			parsed, err := time.Parse(time.RFC3339, *input.TakenAt)
			if err != nil {
				return nil, catalog.Photo{}, errors.New("taken_at must be RFC3339")
			}
			takenAt = &parsed
		}
		photo, err := store.Update(ctx, strings.TrimSpace(input.PhotoID), catalog.PhotoUpdate{Title: input.Title, Project: input.Project, TakenAt: takenAt, Tags: input.Tags, Camera: input.Camera, Lens: input.Lens, Aperture: input.Aperture, ShutterSpeed: input.ShutterSpeed, ISO: input.ISO, FocalLength: input.FocalLength, Location: input.Location, ClearLocation: input.ClearLocation, Description: input.Description, Copyright: input.Copyright, Rating: input.Rating, Favorite: input.Favorite, UserMetadata: input.UserMetadata, Revision: input.Revision})
		return nil, photo, err
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
	additive := false
	mcp.AddTool(server, &mcp.Tool{Name: "update_media_metadata", Title: "Update video or audio metadata", Description: "Update photographer-controlled media fields, tags, or a corrected transcript without changing files, hashes, vectors, or processing state.", Annotations: &mcp.ToolAnnotations{Title: "Update media metadata", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld}}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateMediaInput) (*mcp.CallToolResult, catalog.MediaAsset, error) {
		if !input.Confirmed {
			return nil, catalog.MediaAsset{}, errors.New("confirmed must be true after the user approves the edit")
		}
		if input.Revision == nil {
			return nil, catalog.MediaAsset{}, errors.New("revision is required; call get_media before editing")
		}
		mediaType, err := validMediaType(input.MediaType)
		if err != nil {
			return nil, catalog.MediaAsset{}, err
		}
		var recordedAt *time.Time
		if input.RecordedAt != nil {
			parsed, parseErr := time.Parse(time.RFC3339, *input.RecordedAt)
			if parseErr != nil {
				return nil, catalog.MediaAsset{}, errors.New("recorded_at must be RFC3339")
			}
			recordedAt = &parsed
		}
		asset, err := store.UpdateMedia(ctx, mediaType, strings.TrimSpace(input.AssetID), catalog.MediaUpdate{Title: input.Title, Project: input.Project, RecordedAt: recordedAt, Tags: input.Tags, Description: input.Description, Copyright: input.Copyright, Rating: input.Rating, Favorite: input.Favorite, Transcript: input.Transcript, UserMetadata: input.UserMetadata, Revision: input.Revision})
		return nil, asset, err
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
