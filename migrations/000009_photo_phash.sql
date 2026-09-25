BEGIN;

ALTER TABLE photos
  ADD COLUMN IF NOT EXISTS phash bigint;

CREATE INDEX IF NOT EXISTS photos_phash_idx ON photos(phash) WHERE phash IS NOT NULL;
CREATE INDEX IF NOT EXISTS photos_duplicate_of_idx ON photos(duplicate_of) WHERE duplicate_of IS NOT NULL;
CREATE INDEX IF NOT EXISTS media_assets_duplicate_of_idx ON media_assets(duplicate_of) WHERE duplicate_of IS NOT NULL;

COMMIT;
