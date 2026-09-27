-- +goose Up
-- The routing migration seeded routes from the default Radarr and Sonarr
-- servers of any plugin. Before routing, a media type's requests all went to
-- the plugin that owns the first usable connection by name (enabled, bound to a
-- plugin installation, with an API key, serving the media type), and that
-- plugin picked among its own servers. Where the seeded routes send requests
-- to a server outside that owner (the first connection is Seerr, say), remove
-- them: a media type without routes goes back to that plugin, as before. Routes
-- an admin has saved since are left alone.
-- +goose StatementBegin
WITH owners AS (
    SELECT m.media_type, o.installation_id, o.capability_id
    FROM (VALUES ('movie'), ('series')) AS m (media_type)
    LEFT JOIN LATERAL (
        SELECT i.installation_id, i.capability_id
        FROM request_integrations i
        WHERE i.enabled AND i.capability_id <> '' AND i.installation_id IS NOT NULL
          AND i.api_key_ref <> ''
          AND (cardinality(i.supported_media_types) = 0 OR m.media_type = ANY(i.supported_media_types))
        ORDER BY i.name, i.id
        LIMIT 1
    ) o ON true
)
DELETE FROM request_routes r
USING owners o
WHERE r.media_type = o.media_type
  AND r.id IN ('fallback-' || r.media_type, 'anime-' || r.media_type)
  AND r.updated_at = r.created_at
  AND EXISTS (
    SELECT 1 FROM request_integrations i
    WHERE i.id IN (r.hd_integration_id, r.uhd_integration_id)
      AND (i.installation_id IS DISTINCT FROM o.installation_id
           OR i.capability_id IS DISTINCT FROM o.capability_id));
-- +goose StatementEnd

-- +goose Down
-- The removed routes are not recreated: the plugin that owned the media type
-- routes it again, as it did before routing existed.
SELECT 1;
