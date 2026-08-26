BEGIN;

ALTER TABLE photos ADD COLUMN IF NOT EXISTS content_sha256 text;

UPDATE photos
SET content_sha256 = encode(digest(path, 'sha256'), 'hex')
WHERE content_sha256 IS NULL;

ALTER TABLE photos ALTER COLUMN content_sha256 SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS photos_content_sha256_idx ON photos (content_sha256);

COMMIT;
