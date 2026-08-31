package catalog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProjectStoryAndMediaRelationsIntegration(t *testing.T) {
	databaseURL := os.Getenv("APOFOCUS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("APOFOCUS_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := NewPostgresStore(db)

	projectOne, err := store.CreateProject(ctx, "Integration project one")
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := store.CreateProject(ctx, "Integration project two")
	if err != nil {
		t.Fatal(err)
	}
	story, err := store.CreateStory(ctx, "Integration story")
	if err != nil {
		t.Fatal(err)
	}
	linkedStories, err := store.ReplaceProjectStories(ctx, projectOne.ID, []string{story.ID})
	if err != nil || len(linkedStories) != 1 || linkedStories[0].ID != story.ID {
		t.Fatalf("project-story relationship failed: stories=%+v err=%v", linkedStories, err)
	}
	var parentID, childID, videoID, audioID string
	if err := db.QueryRowContext(ctx, `INSERT INTO photos(title,capture_year,taken_at,path,image_url,thumbnail_url) VALUES('parent',2026,now(),'/integration-parent-'||gen_random_uuid()::text||'.raw','','') RETURNING id::text`).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO photos(title,capture_year,taken_at,path,image_url,thumbnail_url) VALUES('child',2026,now(),'/integration-child-'||gen_random_uuid()::text||'.tiff','','') RETURNING id::text`).Scan(&childID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO media_assets(media_type,title,capture_year,recorded_at,path,media_url) VALUES('video','video',2026,now(),'/integration-video-'||gen_random_uuid()::text||'.mov','') RETURNING id::text`).Scan(&videoID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO media_assets(media_type,title,capture_year,recorded_at,path,media_url) VALUES('audio','audio',2026,now(),'/integration-audio-'||gen_random_uuid()::text||'.wav','') RETURNING id::text`).Scan(&audioID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM photos WHERE id IN ($1,$2)`, parentID, childID)
		_, _ = db.Exec(`DELETE FROM media_assets WHERE id IN ($1,$2)`, videoID, audioID)
		_, _ = db.Exec(`DELETE FROM stories WHERE id=$1`, story.ID)
		_, _ = db.Exec(`DELETE FROM projects WHERE id IN ($1,$2)`, projectOne.ID, projectTwo.ID)
	})

	parent, err := store.Get(ctx, parentID)
	if err != nil {
		t.Fatal(err)
	}
	projectIDs := []string{projectOne.ID, projectTwo.ID}
	storyIDs := []string{story.ID}
	children := []PhotoDerivationInput{{PhotoID: childID, RelationType: "raw_export"}}
	parent, err = store.Update(ctx, parentID, PhotoUpdate{ProjectIDs: &projectIDs, StoryIDs: &storyIDs, Children: &children, Revision: &parent.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Relations.Projects) != 2 || len(parent.Relations.Stories) != 1 || len(parent.Relations.Children) != 1 || parent.Relations.Children[0].RelationType != "raw_export" {
		t.Fatalf("unexpected photo relationships: %+v", parent.Relations)
	}
	child, err := store.Get(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if len(child.Relations.Parents) != 1 || child.Relations.Parents[0].Photo.ID != parentID {
		t.Fatalf("child did not expose its parent: %+v", child.Relations)
	}
	rollbackProjects := []string{projectOne.ID}
	if _, err := store.BulkUpdateRelations(ctx, BulkRelationUpdate{MediaType: "photo", AssetIDs: []string{childID, "00000000-0000-4000-8000-000000000099"}, Operation: "add", ProjectIDs: &rollbackProjects}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected a missing asset to abort the batch, got %v", err)
	}
	var rolledBackCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM project_photos WHERE photo_id=$1`, childID).Scan(&rolledBackCount); err != nil || rolledBackCount != 0 {
		t.Fatalf("failed bulk update was not fully rolled back: count=%d err=%v", rolledBackCount, err)
	}
	bulkProjects := []string{projectTwo.ID}
	bulkStories := []string{}
	result, err := store.BulkUpdateRelations(ctx, BulkRelationUpdate{
		MediaType: "photo", AssetIDs: []string{childID}, Operation: "replace",
		ProjectIDs: &bulkProjects, StoryIDs: &bulkStories,
	})
	if err != nil || result.AssetCount != 1 {
		t.Fatalf("bulk photo relationship replacement failed: result=%+v err=%v", result, err)
	}
	_, err = store.BulkUpdateRelations(ctx, BulkRelationUpdate{
		MediaType: "photo", AssetIDs: []string{childID}, Operation: "add",
		PhotoRelation: &BulkPhotoRelation{Direction: "children_of", OtherPhotoID: parentID, RelationType: "jpeg_export"},
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err = store.Get(ctx, childID)
	if err != nil || len(child.Relations.Projects) != 1 || child.Relations.Projects[0].ID != projectTwo.ID || len(child.Relations.Stories) != 0 || child.Relations.Parents[0].RelationType != "jpeg_export" {
		t.Fatalf("unexpected bulk photo relationships: child=%+v err=%v", child.Relations, err)
	}

	video, err := store.GetMedia(ctx, "video", videoID)
	if err != nil {
		t.Fatal(err)
	}
	relatedIDs := []string{audioID}
	video, err = store.UpdateMedia(ctx, "video", videoID, MediaUpdate{ProjectIDs: &projectIDs, StoryIDs: &storyIDs, RelatedAssetIDs: &relatedIDs, Revision: &video.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(video.Relations.Projects) != 2 || len(video.Relations.Stories) != 1 || len(video.Relations.RelatedMedia) != 1 || video.Relations.RelatedMedia[0].ID != audioID {
		t.Fatalf("unexpected video relationships: %+v", video.Relations)
	}
	audio, err := store.GetMedia(ctx, "audio", audioID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audio.Relations.RelatedMedia) != 1 || audio.Relations.RelatedMedia[0].ID != videoID {
		t.Fatalf("audio did not expose its related video: %+v", audio.Relations)
	}
	bulkMediaProjects := []string{projectOne.ID}
	if _, err := store.BulkUpdateRelations(ctx, BulkRelationUpdate{MediaType: "video", AssetIDs: []string{videoID}, Operation: "replace", ProjectIDs: &bulkMediaProjects}); err != nil {
		t.Fatal(err)
	}
	video, err = store.GetMedia(ctx, "video", videoID)
	if err != nil || len(video.Relations.Projects) != 1 || video.Relations.Projects[0].ID != projectOne.ID {
		t.Fatalf("unexpected bulk video relationships: video=%+v err=%v", video.Relations, err)
	}

	updatedStory, err := store.UpdateStory(ctx, story.ID, "Updated integration story")
	if err != nil || updatedStory.Description != "Updated integration story" {
		t.Fatalf("story update failed: item=%+v err=%v", updatedStory, err)
	}
	catalog, err := store.ListRelationCatalog(ctx)
	if err != nil || len(catalog.Projects) < 2 || len(catalog.Stories) < 1 || len(catalog.ProjectStories) < 1 {
		t.Fatalf("relation catalog failed: catalog=%+v err=%v", catalog, err)
	}
}
