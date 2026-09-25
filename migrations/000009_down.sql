BEGIN;

DROP INDEX IF EXISTS media_assets_duplicate_of_idx;
DROP INDEX IF EXISTS photos_duplicate_of_idx;
DROP INDEX IF EXISTS photos_phash_idx;

ALTER TABLE photos
  DROP COLUMN IF EXISTS phash;

COMMIT;
