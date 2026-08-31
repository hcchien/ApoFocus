package catalog

import "context"

type Project struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type Story struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type PhotoReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type MediaReference struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	Title     string `json:"title"`
}

type PhotoDerivation struct {
	Photo        PhotoReference `json:"photo"`
	RelationType string         `json:"relationType"`
}

type PhotoDerivationInput struct {
	PhotoID      string `json:"photoId"`
	RelationType string `json:"relationType,omitempty"`
}

type PhotoRelations struct {
	Projects []Project         `json:"projects"`
	Stories  []Story           `json:"stories"`
	Parents  []PhotoDerivation `json:"parents"`
	Children []PhotoDerivation `json:"children"`
}

type MediaRelations struct {
	Projects     []Project        `json:"projects"`
	Stories      []Story          `json:"stories"`
	RelatedMedia []MediaReference `json:"relatedMedia"`
}

type RelationCatalog struct {
	Projects       []Project              `json:"projects"`
	Stories        []Story                `json:"stories"`
	ProjectStories []ProjectStoryRelation `json:"projectStories"`
}

type ProjectStoryRelation struct {
	ProjectID string `json:"projectId"`
	StoryID   string `json:"storyId"`
}

type BulkPhotoRelation struct {
	Direction    string `json:"direction"`
	OtherPhotoID string `json:"otherPhotoId"`
	RelationType string `json:"relationType,omitempty"`
}

type BulkRelationUpdate struct {
	MediaType     string             `json:"mediaType"`
	AssetIDs      []string           `json:"assetIds"`
	Operation     string             `json:"operation"`
	ProjectIDs    *[]string          `json:"projectIds,omitempty"`
	StoryIDs      *[]string          `json:"storyIds,omitempty"`
	PhotoRelation *BulkPhotoRelation `json:"photoRelation,omitempty"`
}

type BulkRelationResult struct {
	MediaType  string `json:"mediaType"`
	Operation  string `json:"operation"`
	AssetCount int    `json:"assetCount"`
}

type RelationStore interface {
	ListRelationCatalog(context.Context) (RelationCatalog, error)
	CreateProject(context.Context, string) (Project, error)
	UpdateProject(context.Context, string, string) (Project, error)
	CreateStory(context.Context, string) (Story, error)
	UpdateStory(context.Context, string, string) (Story, error)
	ReplaceProjectStories(context.Context, string, []string) ([]Story, error)
	BulkUpdateRelations(context.Context, BulkRelationUpdate) (BulkRelationResult, error)
}
