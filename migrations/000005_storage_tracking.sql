BEGIN;

CREATE TABLE storage_roots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  base_path text NOT NULL UNIQUE,
  volume_id text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'online' CHECK (status IN ('online', 'offline')),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  last_event_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE photos
  ADD COLUMN storage_root_id uuid REFERENCES storage_roots(id) ON DELETE SET NULL,
  ADD COLUMN relative_path text,
  ADD COLUMN file_id text,
  ADD COLUMN availability_status text NOT NULL DEFAULT 'unknown',
  ADD COLUMN last_verified_at timestamptz,
  ADD COLUMN thumbnail_relative_path text,
  ADD COLUMN thumbnail_file_id text,
  ADD COLUMN thumbnail_status text NOT NULL DEFAULT 'unknown';

ALTER TABLE photos
  ADD CONSTRAINT photos_availability_status_check CHECK (availability_status IN ('unknown', 'available', 'missing', 'volume_offline')),
  ADD CONSTRAINT photos_thumbnail_status_check CHECK (thumbnail_status IN ('unknown', 'available', 'missing', 'volume_offline'));

ALTER TABLE media_assets
  ADD COLUMN storage_root_id uuid REFERENCES storage_roots(id) ON DELETE SET NULL,
  ADD COLUMN relative_path text,
  ADD COLUMN file_id text,
  ADD COLUMN availability_status text NOT NULL DEFAULT 'unknown',
  ADD COLUMN last_verified_at timestamptz,
  ADD COLUMN thumbnail_relative_path text,
  ADD COLUMN thumbnail_file_id text,
  ADD COLUMN thumbnail_status text NOT NULL DEFAULT 'unknown';

ALTER TABLE media_assets
  ADD CONSTRAINT media_assets_availability_status_check CHECK (availability_status IN ('unknown', 'available', 'missing', 'volume_offline')),
  ADD CONSTRAINT media_assets_thumbnail_status_check CHECK (thumbnail_status IN ('unknown', 'available', 'missing', 'volume_offline'));

ALTER TABLE media_segments
  ADD COLUMN keyframe_relative_path text,
  ADD COLUMN keyframe_file_id text,
  ADD COLUMN keyframe_status text NOT NULL DEFAULT 'unknown',
  ADD COLUMN last_verified_at timestamptz;

ALTER TABLE media_segments
  ADD CONSTRAINT media_segments_keyframe_status_check CHECK (keyframe_status IN ('unknown', 'available', 'missing', 'volume_offline'));

CREATE INDEX photos_storage_relative_idx ON photos (storage_root_id, relative_path);
CREATE INDEX photos_storage_file_id_idx ON photos (storage_root_id, file_id) WHERE file_id IS NOT NULL;
CREATE INDEX photos_storage_thumbnail_file_id_idx ON photos (storage_root_id, thumbnail_file_id) WHERE thumbnail_file_id IS NOT NULL;
CREATE INDEX media_assets_storage_relative_idx ON media_assets (storage_root_id, relative_path);
CREATE INDEX media_assets_storage_file_id_idx ON media_assets (storage_root_id, file_id) WHERE file_id IS NOT NULL;
CREATE INDEX media_assets_storage_thumbnail_file_id_idx ON media_assets (storage_root_id, thumbnail_file_id) WHERE thumbnail_file_id IS NOT NULL;
CREATE INDEX media_segments_keyframe_file_id_idx ON media_segments (media_asset_id, keyframe_file_id) WHERE keyframe_file_id IS NOT NULL;

COMMIT;
