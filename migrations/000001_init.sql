BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tags (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE photos (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid REFERENCES projects(id) ON DELETE SET NULL,
  title text NOT NULL,
  capture_year smallint NOT NULL CHECK (capture_year BETWEEN 1800 AND 2200),
  taken_at timestamptz NOT NULL,
  camera text,
  lens text,
  aperture text,
  shutter_speed text,
  iso integer CHECK (iso > 0),
  focal_length text,
  dimensions text,
  file_type text,
  file_size text,
  location_name text,
  latitude double precision CHECK (latitude BETWEEN -90 AND 90),
  longitude double precision CHECK (longitude BETWEEN -180 AND 180),
  path text NOT NULL,
  thumbnail_path text,
  content_sha256 text NOT NULL,
  image_url text NOT NULL,
  thumbnail_url text NOT NULL,
  aspect_ratio text NOT NULL DEFAULT 'landscape',
  dominant_color text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  embedding vector(512),
  search_document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(camera, '') || ' ' || coalesce(lens, '') || ' ' || coalesce(location_name, '')), 'B')
  ) STORED,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((latitude IS NULL) = (longitude IS NULL))
);

CREATE TABLE photo_tags (
  photo_id uuid NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
  tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (photo_id, tag_id)
);

CREATE INDEX photos_capture_year_idx ON photos (capture_year DESC);
CREATE INDEX photos_project_id_idx ON photos (project_id);
CREATE INDEX photos_taken_at_idx ON photos (taken_at DESC);
CREATE INDEX photos_camera_idx ON photos (camera);
CREATE UNIQUE INDEX photos_path_idx ON photos (path);
CREATE UNIQUE INDEX photos_content_sha256_idx ON photos (content_sha256);
CREATE INDEX photos_iso_idx ON photos (iso);
CREATE INDEX photos_search_idx ON photos USING gin (search_document);
CREATE INDEX photo_tags_tag_id_idx ON photo_tags (tag_id, photo_id);
CREATE INDEX photos_embedding_hnsw_idx ON photos USING hnsw (embedding vector_cosine_ops);
CREATE INDEX photos_metadata_gin_idx ON photos USING gin (metadata jsonb_path_ops);

COMMIT;
