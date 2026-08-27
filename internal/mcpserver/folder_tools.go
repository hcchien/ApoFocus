package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
)

type BrowseFoldersInput struct {
	MediaType string `json:"media_type,omitempty" jsonschema:"photo, video, or audio; defaults to photo"`
	Locale    string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de; defaults to en"`
}

type FolderChild struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Kind   string         `json:"kind"`
	Count  int            `json:"count,omitempty"`
	Filter map[string]any `json:"filter,omitempty"`
}

type FolderSource struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Kind     string        `json:"kind"`
	Children []FolderChild `json:"children"`
}

type FolderBrowser struct {
	MediaType   string               `json:"mediaType"`
	Sources     []FolderSource       `json:"sources"`
	Collections []folders.Collection `json:"collections"`
}

type CreateCollectionInput struct {
	Name     string         `json:"name" jsonschema:"collection name"`
	Kind     string         `json:"kind" jsonschema:"manual or smart"`
	ParentID string         `json:"parent_id,omitempty" jsonschema:"optional parent collection UUID"`
	Filter   map[string]any `json:"filter,omitempty" jsonschema:"saved search filter for a smart collection"`
}

type AddCollectionPhotosInput struct {
	CollectionID string   `json:"collection_id" jsonschema:"manual collection UUID"`
	PhotoIDs     []string `json:"photo_ids" jsonschema:"photo UUIDs to add; maximum 200 per call"`
}

type CollectionPhotosInput struct {
	CollectionID string `json:"collection_id" jsonschema:"manual collection UUID"`
	Limit        int    `json:"limit,omitempty" jsonschema:"maximum photos to return from 1 to 200; defaults to 100"`
}

type CollectionPhotosOutput struct {
	CollectionID string          `json:"collectionId"`
	Items        []catalog.Photo `json:"items"`
	Total        int             `json:"total"`
}

type MutationOutput struct {
	Success bool `json:"success"`
	Count   int  `json:"count,omitempty"`
}

func addFolderTools(server *mcp.Server, repository folders.Repository, photos catalog.Store, media catalog.MediaStore) {
	closedWorld, additive := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "browse_folders", Title: "Browse virtual folders", Description: "Browse Finder-like virtual folders derived from year, project, tags and equipment or codec, alongside saved manual and smart collections. This never moves physical files.",
		Annotations: &mcp.ToolAnnotations{Title: "Browse virtual folders", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BrowseFoldersInput) (*mcp.CallToolResult, FolderBrowser, error) {
		result, err := buildFolderBrowser(ctx, input, repository, photos, media)
		return nil, result, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_collection", Title: "Create collection", Description: "Create a manual photo collection or a smart collection backed by a saved filter.",
		Annotations: &mcp.ToolAnnotations{Title: "Create collection", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateCollectionInput) (*mcp.CallToolResult, folders.Collection, error) {
		collection, err := repository.Create(ctx, folders.CreateInput{Name: input.Name, Kind: input.Kind, ParentID: input.ParentID, Filter: input.Filter})
		return nil, collection, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "add_photos_to_collection", Title: "Add photos to collection", Description: "Add up to 200 existing photo IDs to a manual collection. Repeated IDs are ignored.",
		Annotations: &mcp.ToolAnnotations{Title: "Add photos to collection", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input AddCollectionPhotosInput) (*mcp.CallToolResult, MutationOutput, error) {
		if len(input.PhotoIDs) == 0 {
			return nil, MutationOutput{}, errors.New("photo_ids is required")
		}
		if len(input.PhotoIDs) > 200 {
			return nil, MutationOutput{}, errors.New("at most 200 photo_ids are allowed")
		}
		err := repository.AddPhotos(ctx, strings.TrimSpace(input.CollectionID), input.PhotoIDs)
		return nil, MutationOutput{Success: err == nil, Count: len(input.PhotoIDs)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_collection_photos", Title: "Get collection photos", Description: "Return photos in a manual collection without exposing local filesystem paths.",
		Annotations: &mcp.ToolAnnotations{Title: "Get collection photos", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CollectionPhotosInput) (*mcp.CallToolResult, CollectionPhotosOutput, error) {
		ids, err := repository.PhotoIDs(ctx, strings.TrimSpace(input.CollectionID))
		if err != nil {
			return nil, CollectionPhotosOutput{}, err
		}
		total, limit := len(ids), bounded(input.Limit, 100, 1, 200)
		if len(ids) > limit {
			ids = ids[:limit]
		}
		items := make([]catalog.Photo, 0, len(ids))
		for _, id := range ids {
			photo, getErr := photos.Get(ctx, id)
			if getErr == nil {
				items = append(items, photo)
			}
		}
		return nil, CollectionPhotosOutput{CollectionID: input.CollectionID, Items: items, Total: total}, nil
	})
}

func buildFolderBrowser(ctx context.Context, input BrowseFoldersInput, repository folders.Repository, photos catalog.Store, media catalog.MediaStore) (FolderBrowser, error) {
	mediaType := strings.ToLower(strings.TrimSpace(input.MediaType))
	if mediaType == "" {
		mediaType = "photo"
	}
	locale := normalizedLocale(input.Locale)
	var sources []FolderSource
	collections := []folders.Collection{}
	if mediaType == "photo" {
		facets, err := photos.Facets(ctx)
		if err != nil {
			return FolderBrowser{}, err
		}
		sources = []FolderSource{
			facetFolder("years", localized(locale, "拍照年份", "Year taken", "Aufnahmejahr"), "year", facets.Years),
			facetFolder("projects", localized(locale, "專題", "Projects", "Projekte"), "project", facets.Projects),
			facetFolder("tags", "Tags", "tag", facets.Tags),
			facetFolder("cameras", localized(locale, "相機", "Cameras", "Kameras"), "camera", facets.Cameras),
			facetFolder("lenses", localized(locale, "鏡頭", "Lenses", "Objektive"), "lens", facets.Lenses),
		}
		collections, err = repository.List(ctx)
		if err != nil {
			return FolderBrowser{}, err
		}
	} else {
		if media == nil {
			return FolderBrowser{}, errors.New("media catalog is not configured")
		}
		valid, err := validMediaType(mediaType)
		if err != nil {
			return FolderBrowser{}, errors.New("media_type must be photo, video, or audio")
		}
		mediaType = valid
		facets, err := media.MediaFacets(ctx, mediaType)
		if err != nil {
			return FolderBrowser{}, err
		}
		yearName := localized(locale, "錄製年份", "Year recorded", "Aufnahmejahr")
		if mediaType == "video" {
			yearName = localized(locale, "拍攝年份", "Year recorded", "Aufnahmejahr")
		}
		sources = []FolderSource{facetFolder("years", yearName, "year", facets.Years), facetFolder("projects", localized(locale, "專題", "Projects", "Projekte"), "project", facets.Projects), facetFolder("tags", "Tags", "tag", facets.Tags), facetFolder("codecs", "Codecs", "codec", facets.Codecs)}
	}
	return FolderBrowser{MediaType: mediaType, Sources: sources, Collections: collections}, nil
}

func facetFolder(id, name, key string, facets []catalog.FacetCount) FolderSource {
	children := make([]FolderChild, 0, len(facets))
	for _, facet := range facets {
		value := any(facet.Value)
		if key == "year" {
			value = catalog.ParseInt(facet.Value)
		}
		children = append(children, FolderChild{ID: id + ":" + facet.Value, Name: facet.Value, Kind: "virtual", Count: facet.Count, Filter: map[string]any{key: value}})
	}
	return FolderSource{ID: id, Name: name, Kind: "category", Children: children}
}

func normalizedLocale(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	if strings.HasPrefix(value, "zh") {
		return "zh-TW"
	}
	if strings.HasPrefix(value, "de") {
		return "de"
	}
	return "en"
}

func localized(locale, zh, en, de string) string {
	if locale == "zh-TW" {
		return zh
	}
	if locale == "de" {
		return de
	}
	return en
}
