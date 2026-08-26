BEGIN;

CREATE TABLE media_assets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  media_type text NOT NULL CHECK (media_type IN ('video', 'audio')),
  project_id uuid REFERENCES projects(id) ON DELETE SET NULL,
  title text NOT NULL,
  capture_year smallint NOT NULL CHECK (capture_year BETWEEN 1800 AND 2200),
  recorded_at timestamptz NOT NULL,
  duration_ms bigint NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  mime_type text NOT NULL DEFAULT '',
  codec text NOT NULL DEFAULT '',
  dimensions text NOT NULL DEFAULT '',
  sample_rate integer CHECK (sample_rate > 0),
  channels integer CHECK (channels > 0),
  path text NOT NULL,
  thumbnail_path text,
  content_sha256 text NOT NULL,
  media_url text NOT NULL,
  thumbnail_url text NOT NULL DEFAULT '',
  transcript text NOT NULL DEFAULT '',
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  search_document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(transcript, '')), 'B') ||
    setweight(to_tsvector('simple', coalesce(codec, '') || ' ' || coalesce(mime_type, '')), 'C')
  ) STORED,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE media_asset_tags (
  media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  source text NOT NULL DEFAULT 'user' CHECK (source IN ('user', 'visual', 'audio', 'transcript', 'shared')),
  confidence real,
  PRIMARY KEY (media_asset_id, tag_id)
);

CREATE TABLE media_segments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  media_asset_id uuid NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  segment_index integer NOT NULL CHECK (segment_index >= 0),
  segment_type text NOT NULL CHECK (segment_type IN ('visual', 'audio', 'transcript')),
  start_ms bigint NOT NULL CHECK (start_ms >= 0),
  end_ms bigint NOT NULL CHECK (end_ms >= start_ms),
  keyframe_path text,
  keyframe_url text NOT NULL DEFAULT '',
  transcript text NOT NULL DEFAULT '',
  tags jsonb NOT NULL DEFAULT '[]'::jsonb,
  visual_embedding vector(512),
  audio_embedding vector(512),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (media_asset_id, segment_type, segment_index)
);

CREATE UNIQUE INDEX media_assets_path_idx ON media_assets (path);
CREATE UNIQUE INDEX media_assets_content_sha256_idx ON media_assets (content_sha256);
CREATE INDEX media_assets_type_recorded_idx ON media_assets (media_type, recorded_at DESC);
CREATE INDEX media_assets_project_idx ON media_assets (project_id);
CREATE INDEX media_assets_search_idx ON media_assets USING gin (search_document);
CREATE INDEX media_assets_metadata_idx ON media_assets USING gin (metadata jsonb_path_ops);
CREATE INDEX media_asset_tags_tag_idx ON media_asset_tags (tag_id, media_asset_id);
CREATE INDEX media_segments_asset_time_idx ON media_segments (media_asset_id, start_ms, end_ms);
CREATE INDEX media_segments_transcript_idx ON media_segments USING gin (to_tsvector('simple', transcript));
CREATE INDEX media_segments_visual_hnsw_idx ON media_segments USING hnsw (visual_embedding vector_cosine_ops) WHERE visual_embedding IS NOT NULL;
CREATE INDEX media_segments_audio_hnsw_idx ON media_segments USING hnsw (audio_embedding vector_cosine_ops) WHERE audio_embedding IS NOT NULL;

ALTER TABLE batch_jobs
  ADD COLUMN media_types jsonb NOT NULL DEFAULT '["photo", "video", "audio"]'::jsonb;

ALTER TABLE batch_items
  ADD COLUMN media_type text NOT NULL DEFAULT 'photo',
  ADD COLUMN media_asset_id uuid REFERENCES media_assets(id) ON DELETE SET NULL;

ALTER TABLE batch_items
  ADD CONSTRAINT batch_items_media_type_check CHECK (media_type IN ('photo', 'video', 'audio'));

CREATE INDEX batch_items_media_asset_idx ON batch_items (media_asset_id) WHERE media_asset_id IS NOT NULL;

COMMIT;
