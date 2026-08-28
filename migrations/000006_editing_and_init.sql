BEGIN;

ALTER TABLE photos
  ALTER COLUMN content_sha256 DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS copyright text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS rating smallint NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
  ADD COLUMN IF NOT EXISTS favorite boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS user_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS content_hash_status text NOT NULL DEFAULT 'completed'
    CHECK (content_hash_status IN ('pending','running','completed','failed')),
  ADD COLUMN IF NOT EXISTS ai_status text NOT NULL DEFAULT 'completed'
    CHECK (ai_status IN ('pending','running','completed','failed')),
  ADD COLUMN IF NOT EXISTS duplicate_of uuid REFERENCES photos(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS tags_user_edited boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS thumbnail_storage_root_id uuid REFERENCES storage_roots(id) ON DELETE SET NULL;

ALTER TABLE media_assets
  ALTER COLUMN content_sha256 DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS copyright text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS rating smallint NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
  ADD COLUMN IF NOT EXISTS favorite boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS user_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS content_hash_status text NOT NULL DEFAULT 'completed'
    CHECK (content_hash_status IN ('pending','running','completed','failed')),
  ADD COLUMN IF NOT EXISTS ai_status text NOT NULL DEFAULT 'completed'
    CHECK (ai_status IN ('pending','running','completed','failed')),
  ADD COLUMN IF NOT EXISTS deep_index_status text NOT NULL DEFAULT 'completed'
    CHECK (deep_index_status IN ('pending','running','completed','failed','skipped')),
  ADD COLUMN IF NOT EXISTS duplicate_of uuid REFERENCES media_assets(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS tags_user_edited boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS transcript_user_edited boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS thumbnail_storage_root_id uuid REFERENCES storage_roots(id) ON DELETE SET NULL;

ALTER TABLE photo_tags
  ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'user'
    CHECK (source IN ('user','shared','visual_ai','transcript_ai')),
  ADD COLUMN IF NOT EXISTS confidence real;

DROP INDEX IF EXISTS photos_search_idx;
ALTER TABLE photos DROP COLUMN search_document;
ALTER TABLE photos ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
  setweight(to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, '')), 'A') ||
  setweight(to_tsvector('simple', coalesce(camera, '') || ' ' || coalesce(lens, '') || ' ' || coalesce(location_name, '')), 'B')
) STORED;
CREATE INDEX photos_search_idx ON photos USING gin(search_document);

DROP INDEX IF EXISTS media_assets_search_idx;
ALTER TABLE media_assets DROP COLUMN search_document;
ALTER TABLE media_assets ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
  setweight(to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, '')), 'A') ||
  setweight(to_tsvector('simple', coalesce(transcript, '')), 'B') ||
  setweight(to_tsvector('simple', coalesce(codec, '') || ' ' || coalesce(mime_type, '')), 'C')
) STORED;
CREATE INDEX media_assets_search_idx ON media_assets USING gin(search_document);

CREATE TABLE IF NOT EXISTS init_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_root text NOT NULL,
  storage_root_id uuid REFERENCES storage_roots(id) ON DELETE SET NULL,
  project text NOT NULL DEFAULT '',
  tags jsonb NOT NULL DEFAULT '[]'::jsonb,
  recursive boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','scanning','cataloging','photo_ai','media_ai','paused','completed','completed_with_errors','failed','cancelled')),
  discovered_count integer NOT NULL DEFAULT 0,
  photo_count integer NOT NULL DEFAULT 0,
  media_count integer NOT NULL DEFAULT 0,
  cataloged_count integer NOT NULL DEFAULT 0,
  photo_ai_count integer NOT NULL DEFAULT 0,
  media_ai_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  current_path text NOT NULL DEFAULT '',
  error text NOT NULL DEFAULT '',
  pause_requested boolean NOT NULL DEFAULT false,
  cancel_requested boolean NOT NULL DEFAULT false,
  discovery_complete boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  heartbeat_at timestamptz,
  finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS init_runs_claim_idx ON init_runs(status,created_at);

CREATE TABLE IF NOT EXISTS init_items (
  id bigserial PRIMARY KEY,
  run_id uuid NOT NULL REFERENCES init_runs(id) ON DELETE CASCADE,
  source_path text NOT NULL,
  media_type text NOT NULL CHECK (media_type IN ('photo','video','audio')),
  size_bytes bigint NOT NULL DEFAULT 0,
  modified_at timestamptz,
  file_id text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'discovered'
    CHECK (status IN ('discovered','cataloging','cataloged','ai_running','completed','failed')),
  asset_id uuid,
  error text NOT NULL DEFAULT '',
  attempt_count integer NOT NULL DEFAULT 0,
  lease_owner text NOT NULL DEFAULT '',
  lease_expires_at timestamptz,
  cataloged_at timestamptz,
  ai_completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(run_id,source_path)
);

CREATE INDEX IF NOT EXISTS init_items_stage_idx ON init_items(run_id,status,media_type,id);
CREATE INDEX IF NOT EXISTS init_items_lease_idx ON init_items(status,lease_expires_at)
  WHERE status IN ('cataloging','ai_running');

COMMIT;
