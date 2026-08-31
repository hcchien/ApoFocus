BEGIN;

DROP TRIGGER IF EXISTS media_assets_sync_primary_project_relation ON media_assets;
DROP FUNCTION IF EXISTS sync_media_primary_project_relation();
DROP TRIGGER IF EXISTS photos_sync_primary_project_relation ON photos;
DROP FUNCTION IF EXISTS sync_photo_primary_project_relation();
DROP TRIGGER IF EXISTS media_assets_protect_video_audio_relation_types ON media_assets;
DROP FUNCTION IF EXISTS protect_video_audio_relation_types();
DROP TRIGGER IF EXISTS video_audio_relations_validate_types ON video_audio_relations;
DROP FUNCTION IF EXISTS validate_video_audio_relation_types();

DROP TABLE IF EXISTS photo_derivations;
DROP TABLE IF EXISTS video_audio_relations;
DROP TABLE IF EXISTS story_media_assets;
DROP TABLE IF EXISTS story_photos;
DROP TABLE IF EXISTS project_media_assets;
DROP TABLE IF EXISTS project_photos;
DROP TABLE IF EXISTS project_stories;
DROP TABLE IF EXISTS stories;

COMMIT;
