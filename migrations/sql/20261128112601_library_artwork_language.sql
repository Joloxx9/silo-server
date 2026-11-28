-- +goose Up
-- artwork_language picks the language of a library's artwork with text
-- (posters, logos, banners) independently of metadata_language. Empty keeps
-- the old behavior: artwork follows the metadata language.
ALTER TABLE public.media_folders
    ADD COLUMN IF NOT EXISTS artwork_language text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.media_folders DROP COLUMN IF EXISTS artwork_language;
