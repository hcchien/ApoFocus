BEGIN;

-- projects already exists and is used by the current import/search API. Keep its
-- legacy name and timestamps while making description the shared minimum field
-- for both organizational entities.
ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';

CREATE TABLE stories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  description text NOT NULL DEFAULT ''
);

CREATE TABLE project_stories (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
  PRIMARY KEY (project_id, story_id)
);

CREATE INDEX project_stories_story_idx
  ON project_stories (story_id, project_id);

CREATE TABLE project_photos (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  PRIMARY KEY (project_id, photo_id)
);

CREATE INDEX project_photos_photo_idx
  ON project_photos (photo_id, project_id);

CREATE TABLE project_media_assets (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  PRIMARY KEY (project_id, media_asset_id)
);

CREATE INDEX project_media_assets_asset_idx
  ON project_media_assets (media_asset_id, project_id);

CREATE TABLE story_photos (
  story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
  photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  PRIMARY KEY (story_id, photo_id)
);

CREATE INDEX story_photos_photo_idx
  ON story_photos (photo_id, story_id);

CREATE TABLE story_media_assets (
  story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
  media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  PRIMARY KEY (story_id, media_asset_id)
);

CREATE INDEX story_media_assets_asset_idx
  ON story_media_assets (media_asset_id, story_id);

CREATE TABLE video_audio_relations (
  video_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  audio_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  PRIMARY KEY (video_id, audio_id),
  CHECK (video_id <> audio_id)
);

CREATE INDEX video_audio_relations_audio_idx
  ON video_audio_relations (audio_id, video_id);

CREATE FUNCTION validate_video_audio_relation_types()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  video_type text;
  audio_type text;
BEGIN
  SELECT media_type INTO video_type FROM media_assets WHERE id = NEW.video_id;
  SELECT media_type INTO audio_type FROM media_assets WHERE id = NEW.audio_id;

  IF video_type IS DISTINCT FROM 'video' THEN
    RAISE EXCEPTION 'video_audio_relations.video_id must reference a video'
      USING ERRCODE = '23514';
  END IF;
  IF audio_type IS DISTINCT FROM 'audio' THEN
    RAISE EXCEPTION 'video_audio_relations.audio_id must reference an audio asset'
      USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER video_audio_relations_validate_types
BEFORE INSERT OR UPDATE ON video_audio_relations
FOR EACH ROW EXECUTE FUNCTION validate_video_audio_relation_types();

CREATE FUNCTION protect_video_audio_relation_types()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.media_type IS NOT DISTINCT FROM OLD.media_type THEN
    RETURN NEW;
  END IF;
  IF NEW.media_type <> 'video'
     AND EXISTS (SELECT 1 FROM video_audio_relations WHERE video_id = OLD.id) THEN
    RAISE EXCEPTION 'cannot change a related video asset to %', NEW.media_type
      USING ERRCODE = '23514';
  END IF;
  IF NEW.media_type <> 'audio'
     AND EXISTS (SELECT 1 FROM video_audio_relations WHERE audio_id = OLD.id) THEN
    RAISE EXCEPTION 'cannot change a related audio asset to %', NEW.media_type
      USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER media_assets_protect_video_audio_relation_types
BEFORE UPDATE OF media_type ON media_assets
FOR EACH ROW EXECUTE FUNCTION protect_video_audio_relation_types();

CREATE TABLE photo_derivations (
  parent_photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  child_photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  relation_type text NOT NULL DEFAULT 'derivative' CHECK (btrim(relation_type) <> ''),
  PRIMARY KEY (parent_photo_id, child_photo_id),
  CHECK (parent_photo_id <> child_photo_id)
);

CREATE INDEX photo_derivations_child_idx
  ON photo_derivations (child_photo_id, parent_photo_id);

-- Preserve every existing single-project association when moving to the new
-- many-to-many representation.
INSERT INTO project_photos (project_id, photo_id)
SELECT project_id, id FROM photos WHERE project_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO project_media_assets (project_id, media_asset_id)
SELECT project_id, id FROM media_assets WHERE project_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Existing ingest/edit code continues writing the legacy primary project_id.
-- Mirror that one primary association without disturbing any extra M:N links.
CREATE FUNCTION sync_photo_primary_project_relation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' AND NEW.project_id IS NOT NULL THEN
    INSERT INTO project_photos (project_id, photo_id)
    VALUES (NEW.project_id, NEW.id)
    ON CONFLICT DO NOTHING;
  ELSIF TG_OP = 'UPDATE' AND OLD.project_id IS DISTINCT FROM NEW.project_id THEN
    IF OLD.project_id IS NOT NULL THEN
      DELETE FROM project_photos
      WHERE project_id = OLD.project_id AND photo_id = NEW.id;
    END IF;
    IF NEW.project_id IS NOT NULL THEN
      INSERT INTO project_photos (project_id, photo_id)
      VALUES (NEW.project_id, NEW.id)
      ON CONFLICT DO NOTHING;
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER photos_sync_primary_project_relation
AFTER INSERT OR UPDATE OF project_id ON photos
FOR EACH ROW EXECUTE FUNCTION sync_photo_primary_project_relation();

CREATE FUNCTION sync_media_primary_project_relation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' AND NEW.project_id IS NOT NULL THEN
    INSERT INTO project_media_assets (project_id, media_asset_id)
    VALUES (NEW.project_id, NEW.id)
    ON CONFLICT DO NOTHING;
  ELSIF TG_OP = 'UPDATE' AND OLD.project_id IS DISTINCT FROM NEW.project_id THEN
    IF OLD.project_id IS NOT NULL THEN
      DELETE FROM project_media_assets
      WHERE project_id = OLD.project_id AND media_asset_id = NEW.id;
    END IF;
    IF NEW.project_id IS NOT NULL THEN
      INSERT INTO project_media_assets (project_id, media_asset_id)
      VALUES (NEW.project_id, NEW.id)
      ON CONFLICT DO NOTHING;
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER media_assets_sync_primary_project_relation
AFTER INSERT OR UPDATE OF project_id ON media_assets
FOR EACH ROW EXECUTE FUNCTION sync_media_primary_project_relation();

COMMIT;
