BEGIN;

DROP INDEX IF EXISTS media_segments_keyframe_file_id_idx;
DROP INDEX IF EXISTS media_assets_storage_thumbnail_file_id_idx;
DROP INDEX IF EXISTS media_assets_storage_file_id_idx;
DROP INDEX IF EXISTS media_assets_storage_relative_idx;
DROP INDEX IF EXISTS photos_storage_thumbnail_file_id_idx;
DROP INDEX IF EXISTS photos_storage_file_id_idx;
DROP INDEX IF EXISTS photos_storage_relative_idx;

ALTER TABLE media_segments DROP CONSTRAINT IF EXISTS media_segments_keyframe_status_check;
ALTER TABLE media_segments
  DROP COLUMN IF EXISTS last_verified_at,
  DROP COLUMN IF EXISTS keyframe_status,
  DROP COLUMN IF EXISTS keyframe_file_id,
  DROP COLUMN IF EXISTS keyframe_relative_path;

ALTER TABLE media_assets DROP CONSTRAINT IF EXISTS media_assets_thumbnail_status_check;
ALTER TABLE media_assets DROP CONSTRAINT IF EXISTS media_assets_availability_status_check;
ALTER TABLE media_assets
  DROP COLUMN IF EXISTS thumbnail_status,
  DROP COLUMN IF EXISTS thumbnail_file_id,
  DROP COLUMN IF EXISTS thumbnail_relative_path,
  DROP COLUMN IF EXISTS last_verified_at,
  DROP COLUMN IF EXISTS availability_status,
  DROP COLUMN IF EXISTS file_id,
  DROP COLUMN IF EXISTS relative_path,
  DROP COLUMN IF EXISTS storage_root_id;

ALTER TABLE photos DROP CONSTRAINT IF EXISTS photos_thumbnail_status_check;
ALTER TABLE photos DROP CONSTRAINT IF EXISTS photos_availability_status_check;
ALTER TABLE photos
  DROP COLUMN IF EXISTS thumbnail_status,
  DROP COLUMN IF EXISTS thumbnail_file_id,
  DROP COLUMN IF EXISTS thumbnail_relative_path,
  DROP COLUMN IF EXISTS last_verified_at,
  DROP COLUMN IF EXISTS availability_status,
  DROP COLUMN IF EXISTS file_id,
  DROP COLUMN IF EXISTS relative_path,
  DROP COLUMN IF EXISTS storage_root_id;

DROP TABLE IF EXISTS storage_roots;

COMMIT;
