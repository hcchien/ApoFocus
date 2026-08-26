BEGIN;

CREATE TABLE IF NOT EXISTS collections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_id uuid REFERENCES collections(id) ON DELETE CASCADE,
  name text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('manual', 'smart')),
  filter jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS collections_parent_name_idx
  ON collections (COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

CREATE TABLE IF NOT EXISTS collection_photos (
  collection_id uuid NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  position integer NOT NULL DEFAULT 0,
  added_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (collection_id, photo_id)
);

CREATE INDEX IF NOT EXISTS collection_photos_order_idx
  ON collection_photos (collection_id, position, added_at);

CREATE TABLE IF NOT EXISTS batch_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_root text NOT NULL,
  project text NOT NULL DEFAULT '',
  tags jsonb NOT NULL DEFAULT '[]'::jsonb,
  recursive boolean NOT NULL DEFAULT true,
  auto_tags boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'scanning', 'running', 'completed', 'completed_with_errors', 'failed', 'cancelled')),
  discovered_count integer NOT NULL DEFAULT 0,
  processed_count integer NOT NULL DEFAULT 0,
  succeeded_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  current_path text NOT NULL DEFAULT '',
  error text NOT NULL DEFAULT '',
  cancel_requested boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  heartbeat_at timestamptz,
  finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS batch_jobs_claim_idx
  ON batch_jobs (status, created_at);

CREATE TABLE IF NOT EXISTS batch_items (
  id bigserial PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES batch_jobs(id) ON DELETE CASCADE,
  source_path text NOT NULL,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
  photo_id uuid REFERENCES photos(id) ON DELETE SET NULL,
  error text NOT NULL DEFAULT '',
  started_at timestamptz,
  finished_at timestamptz,
  UNIQUE (job_id, source_path)
);

CREATE INDEX IF NOT EXISTS batch_items_progress_idx
  ON batch_items (job_id, status, id);

COMMIT;
