BEGIN;

DROP INDEX IF EXISTS batch_items_media_asset_idx;
ALTER TABLE batch_items DROP CONSTRAINT IF EXISTS batch_items_media_type_check;
ALTER TABLE batch_items DROP COLUMN IF EXISTS media_asset_id;
ALTER TABLE batch_items DROP COLUMN IF EXISTS media_type;
ALTER TABLE batch_jobs DROP COLUMN IF EXISTS media_types;
DROP TABLE IF EXISTS media_segments;
DROP TABLE IF EXISTS media_asset_tags;
DROP TABLE IF EXISTS media_assets;

COMMIT;
