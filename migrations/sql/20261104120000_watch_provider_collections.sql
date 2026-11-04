-- +goose Up
-- +goose StatementBegin

-- Two-way sync of a watch provider account's own named lists with manual
-- collections on the connected profile. Off for existing connections, so
-- nothing creates collections until the profile turns it on; new connections
-- are created with it on by the service.
ALTER TABLE public.watch_provider_connections
    ADD COLUMN sync_collections_enabled boolean NOT NULL DEFAULT false;

-- Which local collection mirrors each provider list. collection_key is the
-- provider's stable list key; collection_id names a collection in the
-- profile's user store, which has no foreign key into this table, so a
-- collection deleted in Silo leaves the link until the next sync replaces it.
-- Item membership state lives in watch_provider_list_items under the list kind
-- 'collection:' || collection_key.
CREATE TABLE public.watch_provider_collection_links (
    connection_id uuid NOT NULL
        REFERENCES public.watch_provider_connections(id) ON DELETE CASCADE,
    collection_key text NOT NULL,
    collection_id text NOT NULL,
    name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (connection_id, collection_key)
);

CREATE INDEX idx_watch_provider_collection_links_collection
    ON public.watch_provider_collection_links (collection_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS public.watch_provider_collection_links;

DELETE FROM public.watch_provider_list_items WHERE list_kind LIKE 'collection:%';

ALTER TABLE public.watch_provider_connections
    DROP COLUMN IF EXISTS sync_collections_enabled;
-- +goose StatementEnd
