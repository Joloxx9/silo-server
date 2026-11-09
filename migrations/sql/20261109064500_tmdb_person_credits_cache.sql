-- +goose Up
-- A person's filmography as TMDB last returned it, so a person page opens
-- from the database after a restart and on every node. Only TMDB data lives
-- here; library presence and request state are worked out per request.
CREATE TABLE tmdb_person_credits_cache (
    person_id integer PRIMARY KEY,
    format smallint NOT NULL,
    credits jsonb NOT NULL,
    fetched_at timestamptz NOT NULL
);
CREATE INDEX tmdb_person_credits_cache_fetched_at ON tmdb_person_credits_cache (fetched_at);

-- +goose Down
DROP TABLE IF EXISTS tmdb_person_credits_cache;
