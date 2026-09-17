-- +goose Up
-- +goose StatementBegin
ALTER TABLE media_folders
    ADD COLUMN IF NOT EXISTS placeholder_episodes_enabled boolean NOT NULL DEFAULT false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE media_folders
    DROP COLUMN IF EXISTS placeholder_episodes_enabled;
-- +goose StatementEnd
