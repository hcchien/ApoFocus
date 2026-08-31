package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *PostgresStore) ListRelationCatalog(ctx context.Context) (RelationCatalog, error) {
	result := RelationCatalog{Projects: []Project{}, Stories: []Story{}, ProjectStories: []ProjectStoryRelation{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id::text,COALESCE(NULLIF(description,''),name,'') FROM projects ORDER BY COALESCE(NULLIF(description,''),name,''),id`)
	if err != nil {
		return result, fmt.Errorf("list projects: %w", err)
	}
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return result, err
		}
		result.Projects = append(result.Projects, item)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT id::text,description FROM stories ORDER BY description,id`)
	if err != nil {
		return result, fmt.Errorf("list stories: %w", err)
	}
	for rows.Next() {
		var item Story
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			return result, err
		}
		result.Stories = append(result.Stories, item)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT project_id::text,story_id::text FROM project_stories ORDER BY project_id,story_id`)
	if err != nil {
		return result, fmt.Errorf("list project story relationships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item ProjectStoryRelation
		if err := rows.Scan(&item.ProjectID, &item.StoryID); err != nil {
			return result, err
		}
		result.ProjectStories = append(result.ProjectStories, item)
	}
	return result, rows.Err()
}

func (s *PostgresStore) CreateProject(ctx context.Context, description string) (Project, error) {
	description = strings.TrimSpace(description)
	var item Project
	err := s.db.QueryRowContext(ctx, `INSERT INTO projects(name,description) VALUES($1,$1)
		ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description
		RETURNING id::text,description`, description).Scan(&item.ID, &item.Description)
	return item, err
}

func (s *PostgresStore) UpdateProject(ctx context.Context, id, description string) (Project, error) {
	var item Project
	err := s.db.QueryRowContext(ctx, `UPDATE projects SET description=$2 WHERE id=$1 RETURNING id::text,description`, id, strings.TrimSpace(description)).Scan(&item.ID, &item.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return item, err
}

func (s *PostgresStore) CreateStory(ctx context.Context, description string) (Story, error) {
	var item Story
	err := s.db.QueryRowContext(ctx, `INSERT INTO stories(description) VALUES($1) RETURNING id::text,description`, strings.TrimSpace(description)).Scan(&item.ID, &item.Description)
	return item, err
}

func (s *PostgresStore) UpdateStory(ctx context.Context, id, description string) (Story, error) {
	var item Story
	err := s.db.QueryRowContext(ctx, `UPDATE stories SET description=$2 WHERE id=$1 RETURNING id::text,description`, id, strings.TrimSpace(description)).Scan(&item.ID, &item.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Story{}, ErrNotFound
	}
	return item, err
}

func (s *PostgresStore) ReplaceProjectStories(ctx context.Context, projectID string, storyIDs []string) ([]Story, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, projectID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	if err := replaceIDLinks(ctx, tx, `DELETE FROM project_stories WHERE project_id=$1`, `INSERT INTO project_stories(project_id,story_id) VALUES($1,$2)`, projectID, storyIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.id::text,s.description FROM project_stories ps JOIN stories s ON s.id=ps.story_id WHERE ps.project_id=$1 ORDER BY s.description,s.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Story{}
	for rows.Next() {
		var item Story
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) loadPhotoRelations(ctx context.Context, photo *Photo) error {
	relations := PhotoRelations{Projects: []Project{}, Stories: []Story{}, Parents: []PhotoDerivation{}, Children: []PhotoDerivation{}}
	rows, err := s.db.QueryContext(ctx, `SELECT p.id::text,COALESCE(NULLIF(p.description,''),p.name,'')
		FROM project_photos pp JOIN projects p ON p.id=pp.project_id WHERE pp.photo_id=$1
		ORDER BY COALESCE(NULLIF(p.description,''),p.name,''),p.id`, photo.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return err
		}
		relations.Projects = append(relations.Projects, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT s.id::text,s.description FROM story_photos sp JOIN stories s ON s.id=sp.story_id WHERE sp.photo_id=$1 ORDER BY s.description,s.id`, photo.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item Story
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return err
		}
		relations.Stories = append(relations.Stories, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	parents, err := s.photoDerivations(ctx, photo.ID, true)
	if err != nil {
		return err
	}
	children, err := s.photoDerivations(ctx, photo.ID, false)
	if err != nil {
		return err
	}
	relations.Parents, relations.Children = parents, children
	photo.Relations = relations
	if len(relations.Projects) > 0 {
		photo.Project = relations.Projects[0].Description
	}
	return nil
}

func (s *PostgresStore) photoDerivations(ctx context.Context, photoID string, parents bool) ([]PhotoDerivation, error) {
	query := `SELECT p.id::text,p.title,pd.relation_type FROM photo_derivations pd JOIN photos p ON p.id=pd.parent_photo_id WHERE pd.child_photo_id=$1 ORDER BY p.title,p.id`
	if !parents {
		query = `SELECT p.id::text,p.title,pd.relation_type FROM photo_derivations pd JOIN photos p ON p.id=pd.child_photo_id WHERE pd.parent_photo_id=$1 ORDER BY p.title,p.id`
	}
	rows, err := s.db.QueryContext(ctx, query, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PhotoDerivation{}
	for rows.Next() {
		var item PhotoDerivation
		if err := rows.Scan(&item.Photo.ID, &item.Photo.Title, &item.RelationType); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *PostgresStore) loadMediaRelations(ctx context.Context, asset *MediaAsset) error {
	relations := MediaRelations{Projects: []Project{}, Stories: []Story{}, RelatedMedia: []MediaReference{}}
	rows, err := s.db.QueryContext(ctx, `SELECT p.id::text,COALESCE(NULLIF(p.description,''),p.name,'') FROM project_media_assets pm JOIN projects p ON p.id=pm.project_id WHERE pm.media_asset_id=$1 ORDER BY COALESCE(NULLIF(p.description,''),p.name,''),p.id`, asset.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return err
		}
		relations.Projects = append(relations.Projects, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT s.id::text,s.description FROM story_media_assets sm JOIN stories s ON s.id=sm.story_id WHERE sm.media_asset_id=$1 ORDER BY s.description,s.id`, asset.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item Story
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return err
		}
		relations.Stories = append(relations.Stories, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	query := `SELECT m.id::text,m.media_type,m.title FROM video_audio_relations r JOIN media_assets m ON m.id=r.audio_id WHERE r.video_id=$1 ORDER BY m.title,m.id`
	if asset.MediaType == "audio" {
		query = `SELECT m.id::text,m.media_type,m.title FROM video_audio_relations r JOIN media_assets m ON m.id=r.video_id WHERE r.audio_id=$1 ORDER BY m.title,m.id`
	}
	rows, err = s.db.QueryContext(ctx, query, asset.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item MediaReference
		if err := rows.Scan(&item.ID, &item.MediaType, &item.Title); err != nil {
			return err
		}
		relations.RelatedMedia = append(relations.RelatedMedia, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	asset.Relations = relations
	if len(relations.Projects) > 0 {
		asset.Project = relations.Projects[0].Description
	}
	return nil
}

func replacePhotoRelations(ctx context.Context, tx *sql.Tx, photoID string, input PhotoUpdate) error {
	if input.ProjectIDs != nil {
		if err := replaceIDLinks(ctx, tx, `DELETE FROM project_photos WHERE photo_id=$1`, `INSERT INTO project_photos(project_id,photo_id) VALUES($2,$1)`, photoID, *input.ProjectIDs); err != nil {
			return fmt.Errorf("replace photo projects: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photos SET project_id=(SELECT project_id FROM project_photos WHERE photo_id=$1 ORDER BY project_id LIMIT 1) WHERE id=$1`, photoID); err != nil {
			return fmt.Errorf("sync photo primary project: %w", err)
		}
	}
	if input.StoryIDs != nil {
		if err := replaceIDLinks(ctx, tx, `DELETE FROM story_photos WHERE photo_id=$1`, `INSERT INTO story_photos(story_id,photo_id) VALUES($2,$1)`, photoID, *input.StoryIDs); err != nil {
			return fmt.Errorf("replace photo stories: %w", err)
		}
	}
	if input.Parents != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM photo_derivations WHERE child_photo_id=$1`, photoID); err != nil {
			return err
		}
		for _, relation := range cleanDerivations(*input.Parents) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO photo_derivations(parent_photo_id,child_photo_id,relation_type) VALUES($2,$1,$3)`, photoID, relation.PhotoID, relation.RelationType); err != nil {
				return err
			}
		}
	}
	if input.Children != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM photo_derivations WHERE parent_photo_id=$1`, photoID); err != nil {
			return err
		}
		for _, relation := range cleanDerivations(*input.Children) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO photo_derivations(parent_photo_id,child_photo_id,relation_type) VALUES($1,$2,$3)`, photoID, relation.PhotoID, relation.RelationType); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceMediaRelations(ctx context.Context, tx *sql.Tx, assetID, mediaType string, input MediaUpdate) error {
	if input.ProjectIDs != nil {
		if err := replaceIDLinks(ctx, tx, `DELETE FROM project_media_assets WHERE media_asset_id=$1`, `INSERT INTO project_media_assets(project_id,media_asset_id) VALUES($2,$1)`, assetID, *input.ProjectIDs); err != nil {
			return fmt.Errorf("replace media projects: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_assets SET project_id=(SELECT project_id FROM project_media_assets WHERE media_asset_id=$1 ORDER BY project_id LIMIT 1) WHERE id=$1`, assetID); err != nil {
			return fmt.Errorf("sync media primary project: %w", err)
		}
	}
	if input.StoryIDs != nil {
		if err := replaceIDLinks(ctx, tx, `DELETE FROM story_media_assets WHERE media_asset_id=$1`, `INSERT INTO story_media_assets(story_id,media_asset_id) VALUES($2,$1)`, assetID, *input.StoryIDs); err != nil {
			return fmt.Errorf("replace media stories: %w", err)
		}
	}
	if input.RelatedAssetIDs != nil {
		deleteQuery := `DELETE FROM video_audio_relations WHERE video_id=$1`
		insertQuery := `INSERT INTO video_audio_relations(video_id,audio_id) VALUES($1,$2)`
		if mediaType == "audio" {
			deleteQuery = `DELETE FROM video_audio_relations WHERE audio_id=$1`
			insertQuery = `INSERT INTO video_audio_relations(video_id,audio_id) VALUES($2,$1)`
		}
		if err := replaceIDLinks(ctx, tx, deleteQuery, insertQuery, assetID, *input.RelatedAssetIDs); err != nil {
			return fmt.Errorf("replace related media: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) BulkUpdateRelations(ctx context.Context, input BulkRelationUpdate) (BulkRelationResult, error) {
	input.MediaType = strings.TrimSpace(input.MediaType)
	input.Operation = strings.TrimSpace(input.Operation)
	input.AssetIDs = cleanIDs(input.AssetIDs)
	result := BulkRelationResult{MediaType: input.MediaType, Operation: input.Operation, AssetCount: len(input.AssetIDs)}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, assetID := range input.AssetIDs {
		var exists bool
		if input.MediaType == "photo" {
			err = tx.QueryRowContext(ctx, `SELECT true FROM photos WHERE id=$1 FOR UPDATE`, assetID).Scan(&exists)
		} else {
			err = tx.QueryRowContext(ctx, `SELECT true FROM media_assets WHERE id=$1 AND media_type=$2 FOR UPDATE`, assetID, input.MediaType).Scan(&exists)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return result, ErrNotFound
		}
		if err != nil {
			return result, err
		}
	}

	for _, assetID := range input.AssetIDs {
		if input.MediaType == "photo" {
			if input.ProjectIDs != nil {
				if err := mutateIDLinks(ctx, tx, input.Operation, `DELETE FROM project_photos WHERE photo_id=$1`, `DELETE FROM project_photos WHERE photo_id=$1 AND project_id=$2`, `INSERT INTO project_photos(photo_id,project_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, assetID, *input.ProjectIDs); err != nil {
					return result, fmt.Errorf("bulk update photo projects: %w", err)
				}
			}
			if input.StoryIDs != nil {
				if err := mutateIDLinks(ctx, tx, input.Operation, `DELETE FROM story_photos WHERE photo_id=$1`, `DELETE FROM story_photos WHERE photo_id=$1 AND story_id=$2`, `INSERT INTO story_photos(photo_id,story_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, assetID, *input.StoryIDs); err != nil {
					return result, fmt.Errorf("bulk update photo stories: %w", err)
				}
			}
		} else {
			if input.ProjectIDs != nil {
				if err := mutateIDLinks(ctx, tx, input.Operation, `DELETE FROM project_media_assets WHERE media_asset_id=$1`, `DELETE FROM project_media_assets WHERE media_asset_id=$1 AND project_id=$2`, `INSERT INTO project_media_assets(media_asset_id,project_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, assetID, *input.ProjectIDs); err != nil {
					return result, fmt.Errorf("bulk update media projects: %w", err)
				}
			}
			if input.StoryIDs != nil {
				if err := mutateIDLinks(ctx, tx, input.Operation, `DELETE FROM story_media_assets WHERE media_asset_id=$1`, `DELETE FROM story_media_assets WHERE media_asset_id=$1 AND story_id=$2`, `INSERT INTO story_media_assets(media_asset_id,story_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, assetID, *input.StoryIDs); err != nil {
					return result, fmt.Errorf("bulk update media stories: %w", err)
				}
			}
		}
	}

	if input.PhotoRelation != nil {
		if err := bulkPhotoDerivations(ctx, tx, input.Operation, input.AssetIDs, *input.PhotoRelation); err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE photos SET revision=revision+1,updated_at=now() WHERE id=$1`, strings.TrimSpace(input.PhotoRelation.OtherPhotoID)); err != nil {
			return result, err
		}
	}
	for _, assetID := range input.AssetIDs {
		if input.MediaType == "photo" {
			_, err = tx.ExecContext(ctx, `UPDATE photos SET project_id=(SELECT project_id FROM project_photos WHERE photo_id=$1 ORDER BY project_id LIMIT 1),revision=revision+1,updated_at=now() WHERE id=$1`, assetID)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE media_assets SET project_id=(SELECT project_id FROM project_media_assets WHERE media_asset_id=$1 ORDER BY project_id LIMIT 1),revision=revision+1,updated_at=now() WHERE id=$1`, assetID)
		}
		if err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func mutateIDLinks(ctx context.Context, tx *sql.Tx, operation, deleteAll, deleteOne, insertOne, ownerID string, values []string) error {
	values = cleanIDs(values)
	if operation == "replace" {
		if _, err := tx.ExecContext(ctx, deleteAll, ownerID); err != nil {
			return err
		}
	}
	for _, value := range values {
		query := insertOne
		if operation == "remove" {
			query = deleteOne
		}
		if _, err := tx.ExecContext(ctx, query, ownerID, value); err != nil {
			return err
		}
	}
	return nil
}

func bulkPhotoDerivations(ctx context.Context, tx *sql.Tx, operation string, photoIDs []string, relation BulkPhotoRelation) error {
	relation.OtherPhotoID = strings.TrimSpace(relation.OtherPhotoID)
	relation.RelationType = strings.TrimSpace(relation.RelationType)
	if relation.RelationType == "" {
		relation.RelationType = "derivative"
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photos WHERE id=$1)`, relation.OtherPhotoID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	for _, photoID := range photoIDs {
		parentID, childID := relation.OtherPhotoID, photoID
		if relation.Direction == "parents_of" {
			parentID, childID = photoID, relation.OtherPhotoID
		}
		if operation == "remove" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM photo_derivations WHERE parent_photo_id=$1 AND child_photo_id=$2`, parentID, childID); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `INSERT INTO photo_derivations(parent_photo_id,child_photo_id,relation_type) VALUES($1,$2,$3) ON CONFLICT(parent_photo_id,child_photo_id) DO UPDATE SET relation_type=EXCLUDED.relation_type`, parentID, childID, relation.RelationType); err != nil {
			return err
		}
	}
	return nil
}

func cleanIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func replaceIDLinks(ctx context.Context, tx *sql.Tx, deleteQuery, insertQuery, ownerID string, values []string) error {
	if _, err := tx.ExecContext(ctx, deleteQuery, ownerID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		if _, err := tx.ExecContext(ctx, insertQuery, ownerID, value); err != nil {
			return err
		}
	}
	return nil
}

func cleanDerivations(values []PhotoDerivationInput) []PhotoDerivationInput {
	result := make([]PhotoDerivationInput, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.PhotoID = strings.TrimSpace(value.PhotoID)
		value.RelationType = strings.TrimSpace(value.RelationType)
		if value.RelationType == "" {
			value.RelationType = "derivative"
		}
		if value.PhotoID != "" && !seen[value.PhotoID] {
			seen[value.PhotoID] = true
			result = append(result, value)
		}
	}
	return result
}
