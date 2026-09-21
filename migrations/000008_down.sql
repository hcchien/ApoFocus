BEGIN;
DROP TABLE IF EXISTS deep_analysis_items;
DROP TABLE IF EXISTS deep_analysis_jobs;
ALTER TABLE photos
  DROP COLUMN IF EXISTS deep_analyzed_at,
  DROP COLUMN IF EXISTS deep_analysis_prompt_version,
  DROP COLUMN IF EXISTS deep_analysis_model,
  DROP COLUMN IF EXISTS deep_analysis,
  DROP COLUMN IF EXISTS deep_analysis_status;
COMMIT;
