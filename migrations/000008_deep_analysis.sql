BEGIN;

ALTER TABLE photos
  ADD COLUMN IF NOT EXISTS deep_analysis_status text NOT NULL DEFAULT 'not_requested'
    CHECK (deep_analysis_status IN ('not_requested','pending','running','completed','failed')),
  ADD COLUMN IF NOT EXISTS deep_analysis jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS deep_analysis_model text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS deep_analysis_prompt_version text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS deep_analyzed_at timestamptz;

CREATE TABLE IF NOT EXISTS deep_analysis_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  model text NOT NULL,
  prompt_version text NOT NULL,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','completed_with_errors','failed','cancelled')),
  selected_count integer NOT NULL DEFAULT 0,
  processed_count integer NOT NULL DEFAULT 0,
  succeeded_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  cancel_requested boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  heartbeat_at timestamptz,
  finished_at timestamptz
);

CREATE INDEX IF NOT EXISTS deep_analysis_jobs_claim_idx
  ON deep_analysis_jobs(status, created_at);

CREATE TABLE IF NOT EXISTS deep_analysis_items (
  id bigserial PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES deep_analysis_jobs(id) ON DELETE CASCADE,
  photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  source_revision bigint NOT NULL,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','completed','failed')),
  result jsonb NOT NULL DEFAULT '{}'::jsonb,
  error text NOT NULL DEFAULT '',
  attempt_count integer NOT NULL DEFAULT 0,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(job_id, photo_id)
);

CREATE INDEX IF NOT EXISTS deep_analysis_items_job_idx
  ON deep_analysis_items(job_id, status, id);

CREATE INDEX IF NOT EXISTS deep_analysis_items_photo_idx
  ON deep_analysis_items(photo_id, created_at DESC);

COMMIT;
