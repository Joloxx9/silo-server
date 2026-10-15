-- +goose Up
-- When Sonarr/Radarr reported deleting this file itself (an upgrade or a
-- manual delete) and Silo saw it gone from disk. A missing file with this set
-- skips scanner.file_removal_grace on the next trash sweep. Cleared whenever
-- the file is seen on disk again. NULL: no such report.
ALTER TABLE media_files ADD COLUMN IF NOT EXISTS removal_confirmed_at timestamptz;

-- +goose Down
ALTER TABLE media_files DROP COLUMN IF EXISTS removal_confirmed_at;
