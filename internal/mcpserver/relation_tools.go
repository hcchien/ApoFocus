package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/catalog"
)

type ListRelationsInput struct{}

type CreateRelationEntityInput struct {
	Description string `json:"description" jsonschema:"human-readable project or story description"`
	Confirmed   bool   `json:"confirmed" jsonschema:"must be true after the user approves creating this entity"`
}

type UpdateRelationEntityInput struct {
	ID          string `json:"id" jsonschema:"project or story UUID"`
	Description string `json:"description" jsonschema:"complete replacement description"`
	Confirmed   bool   `json:"confirmed" jsonschema:"must be true after the user approves this edit"`
}

type UpdateProjectStoriesInput struct {
	ProjectID string   `json:"project_id" jsonschema:"project UUID"`
	StoryIDs  []string `json:"story_ids" jsonschema:"complete replacement list of story UUIDs"`
	Confirmed bool     `json:"confirmed" jsonschema:"must be true after the user approves replacing these relationships"`
}

type BulkPhotoRelationInput struct {
	Direction    string `json:"direction" jsonschema:"children_of means selected photos are children of other_photo_id; parents_of means selected photos are parents of it"`
	OtherPhotoID string `json:"other_photo_id" jsonschema:"the single photo UUID related to every selected photo"`
	RelationType string `json:"relation_type,omitempty" jsonschema:"relationship label such as raw_export, crop, tiff_export, or jpeg_export"`
}

type BulkUpdateRelationsInput struct {
	MediaType     string                  `json:"media_type" jsonschema:"photo, video, or audio"`
	AssetIDs      []string                `json:"asset_ids" jsonschema:"UUIDs selected from search results; maximum 500"`
	Operation     string                  `json:"operation" jsonschema:"add, remove, or replace; photo_relation supports add and remove only"`
	ApplyProjects bool                    `json:"apply_projects,omitempty" jsonschema:"true to apply project_ids; required to intentionally replace projects with an empty list"`
	ProjectIDs    []string                `json:"project_ids,omitempty" jsonschema:"project UUIDs to add, remove, or use as complete replacement"`
	ApplyStories  bool                    `json:"apply_stories,omitempty" jsonschema:"true to apply story_ids; required to intentionally replace stories with an empty list"`
	StoryIDs      []string                `json:"story_ids,omitempty" jsonschema:"story UUIDs to add, remove, or use as complete replacement"`
	PhotoRelation *BulkPhotoRelationInput `json:"photo_relation,omitempty" jsonschema:"optional parent-child change applied to every selected photo"`
	Confirmed     bool                    `json:"confirmed" jsonschema:"must be true after the user approves the selected assets and relationship operation"`
}

func addRelationTools(server *mcp.Server, store catalog.RelationStore) {
	readOnly, closedWorld, additive := true, false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_projects_and_stories", Title: "List projects and stories",
		Description: "List the project and story IDs and descriptions available for linking to photos, videos, and audio.",
		Annotations: &mcp.ToolAnnotations{Title: "List projects and stories", ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ ListRelationsInput) (*mcp.CallToolResult, catalog.RelationCatalog, error) {
		result, err := store.ListRelationCatalog(ctx)
		return nil, result, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_project", Title: "Create project", Description: "Create a project that can be linked to any number of stories and media assets. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Create project", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateRelationEntityInput) (*mcp.CallToolResult, catalog.Project, error) {
		if err := validateRelationEntityInput(input.Description, input.Confirmed); err != nil {
			return nil, catalog.Project{}, err
		}
		item, err := store.CreateProject(ctx, strings.TrimSpace(input.Description))
		return nil, item, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_project", Title: "Update project", Description: "Replace a project's description without changing its relationships. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Update project", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateRelationEntityInput) (*mcp.CallToolResult, catalog.Project, error) {
		if err := validateRelationEntityUpdate(input); err != nil {
			return nil, catalog.Project{}, err
		}
		item, err := store.UpdateProject(ctx, strings.TrimSpace(input.ID), strings.TrimSpace(input.Description))
		return nil, item, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "create_story", Title: "Create story", Description: "Create a story that can be linked to any number of projects and media assets. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Create story", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateRelationEntityInput) (*mcp.CallToolResult, catalog.Story, error) {
		if err := validateRelationEntityInput(input.Description, input.Confirmed); err != nil {
			return nil, catalog.Story{}, err
		}
		item, err := store.CreateStory(ctx, strings.TrimSpace(input.Description))
		return nil, item, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_story", Title: "Update story", Description: "Replace a story's description without changing its relationships. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Update story", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateRelationEntityInput) (*mcp.CallToolResult, catalog.Story, error) {
		if err := validateRelationEntityUpdate(input); err != nil {
			return nil, catalog.Story{}, err
		}
		item, err := store.UpdateStory(ctx, strings.TrimSpace(input.ID), strings.TrimSpace(input.Description))
		return nil, item, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_project_story_relationships", Title: "Update project story relationships", Description: "Replace the complete set of stories linked to one project. An empty story_ids list clears the relationships. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Update project story relationships", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateProjectStoriesInput) (*mcp.CallToolResult, []catalog.Story, error) {
		if !input.Confirmed {
			return nil, nil, errors.New("confirmed must be true after the user approves the relationship change")
		}
		if strings.TrimSpace(input.ProjectID) == "" {
			return nil, nil, errors.New("project_id is required")
		}
		items, err := store.ReplaceProjectStories(ctx, strings.TrimSpace(input.ProjectID), input.StoryIDs)
		return nil, items, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "bulk_update_asset_relationships", Title: "Bulk update asset relationships",
		Description: "Atomically add, remove, or replace Project/Story relationships for up to 500 searched photo, video, or audio IDs. For photos, it can also add or remove one parent-child relationship against every selected photo. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "Bulk update asset relationships", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BulkUpdateRelationsInput) (*mcp.CallToolResult, catalog.BulkRelationResult, error) {
		update, err := bulkRelationUpdate(input)
		if err != nil {
			return nil, catalog.BulkRelationResult{}, err
		}
		result, err := store.BulkUpdateRelations(ctx, update)
		return nil, result, err
	})
}

func bulkRelationUpdate(input BulkUpdateRelationsInput) (catalog.BulkRelationUpdate, error) {
	if !input.Confirmed {
		return catalog.BulkRelationUpdate{}, errors.New("confirmed must be true after the user approves the bulk relationship change")
	}
	if len(input.AssetIDs) == 0 || len(input.AssetIDs) > 500 {
		return catalog.BulkRelationUpdate{}, errors.New("asset_ids must contain between 1 and 500 UUIDs")
	}
	if input.MediaType != "photo" && input.MediaType != "video" && input.MediaType != "audio" {
		return catalog.BulkRelationUpdate{}, errors.New("media_type must be photo, video, or audio")
	}
	if input.Operation != "add" && input.Operation != "remove" && input.Operation != "replace" {
		return catalog.BulkRelationUpdate{}, errors.New("operation must be add, remove, or replace")
	}
	result := catalog.BulkRelationUpdate{MediaType: input.MediaType, AssetIDs: input.AssetIDs, Operation: input.Operation}
	if input.ApplyProjects {
		result.ProjectIDs = &input.ProjectIDs
	}
	if input.ApplyStories {
		result.StoryIDs = &input.StoryIDs
	}
	if input.PhotoRelation != nil {
		if input.MediaType != "photo" || input.Operation == "replace" {
			return catalog.BulkRelationUpdate{}, errors.New("photo_relation requires media_type photo and operation add or remove")
		}
		if input.PhotoRelation.Direction != "children_of" && input.PhotoRelation.Direction != "parents_of" {
			return catalog.BulkRelationUpdate{}, errors.New("photo_relation.direction must be children_of or parents_of")
		}
		if strings.TrimSpace(input.PhotoRelation.OtherPhotoID) == "" {
			return catalog.BulkRelationUpdate{}, errors.New("photo_relation.other_photo_id is required")
		}
		result.PhotoRelation = &catalog.BulkPhotoRelation{Direction: input.PhotoRelation.Direction, OtherPhotoID: strings.TrimSpace(input.PhotoRelation.OtherPhotoID), RelationType: strings.TrimSpace(input.PhotoRelation.RelationType)}
	}
	if result.ProjectIDs == nil && result.StoryIDs == nil && result.PhotoRelation == nil {
		return catalog.BulkRelationUpdate{}, errors.New("select at least one relationship type to apply")
	}
	return result, nil
}

func validateRelationEntityInput(description string, confirmed bool) error {
	if !confirmed {
		return errors.New("confirmed must be true after the user approves the change")
	}
	if strings.TrimSpace(description) == "" {
		return errors.New("description is required")
	}
	if len([]rune(description)) > 10000 {
		return errors.New("description is too long")
	}
	return nil
}

func validateRelationEntityUpdate(input UpdateRelationEntityInput) error {
	if strings.TrimSpace(input.ID) == "" {
		return errors.New("id is required")
	}
	return validateRelationEntityInput(input.Description, input.Confirmed)
}
