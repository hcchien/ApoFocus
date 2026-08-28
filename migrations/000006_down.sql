BEGIN;
DROP TABLE IF EXISTS init_items;
DROP TABLE IF EXISTS init_runs;
ALTER TABLE photo_tags DROP COLUMN IF EXISTS confidence, DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS photos_search_idx;
ALTER TABLE photos DROP COLUMN search_document;
DROP INDEX IF EXISTS media_assets_search_idx;
ALTER TABLE media_assets DROP COLUMN search_document;
ALTER TABLE media_assets
  DROP COLUMN IF EXISTS thumbnail_storage_root_id, DROP COLUMN IF EXISTS transcript_user_edited, DROP COLUMN IF EXISTS tags_user_edited,
  DROP COLUMN IF EXISTS duplicate_of, DROP COLUMN IF EXISTS deep_index_status,
  DROP COLUMN IF EXISTS ai_status, DROP COLUMN IF EXISTS content_hash_status,
  DROP COLUMN IF EXISTS revision, DROP COLUMN IF EXISTS user_metadata,
  DROP COLUMN IF EXISTS favorite, DROP COLUMN IF EXISTS rating,
  DROP COLUMN IF EXISTS copyright, DROP COLUMN IF EXISTS description;
ALTER TABLE photos
  DROP COLUMN IF EXISTS thumbnail_storage_root_id, DROP COLUMN IF EXISTS tags_user_edited, DROP COLUMN IF EXISTS duplicate_of, DROP COLUMN IF EXISTS ai_status,
  DROP COLUMN IF EXISTS content_hash_status, DROP COLUMN IF EXISTS revision,
  DROP COLUMN IF EXISTS user_metadata, DROP COLUMN IF EXISTS favorite,
  DROP COLUMN IF EXISTS rating, DROP COLUMN IF EXISTS copyright,
  DROP COLUMN IF EXISTS description;
ALTER TABLE photos ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
  setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
  setweight(to_tsvector('simple', coalesce(camera, '') || ' ' || coalesce(lens, '') || ' ' || coalesce(location_name, '')), 'B')
) STORED;
CREATE INDEX photos_search_idx ON photos USING gin(search_document);
ALTER TABLE media_assets ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
  setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
  setweight(to_tsvector('simple', coalesce(transcript, '')), 'B') ||
  setweight(to_tsvector('simple', coalesce(codec, '') || ' ' || coalesce(mime_type, '')), 'C')
) STORED;
CREATE INDEX media_assets_search_idx ON media_assets USING gin(search_document);
COMMIT;
