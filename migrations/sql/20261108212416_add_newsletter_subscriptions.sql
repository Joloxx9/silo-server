-- +goose Up
CREATE TABLE notification_delivery_preferences (
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id text NOT NULL,
    preferences jsonb NOT NULL DEFAULT '{}',
    alerts_since timestamptz NOT NULL DEFAULT now(),
    newsletter_since timestamptz NOT NULL DEFAULT now(),
    alerts_sent_at timestamptz,
    newsletter_sent_at timestamptz,
    attempted_at timestamptz,
    failures integer NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, profile_id)
);
CREATE INDEX notification_delivery_preferences_attempted ON notification_delivery_preferences (attempted_at NULLS FIRST);
CREATE TABLE notification_newsletter_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false,
    settings jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE notification_newsletter_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id text NOT NULL,
    channel text NOT NULL CHECK (channel IN ('alerts', 'newsletter')),
    sent_at timestamptz NOT NULL DEFAULT now(),
    item_count integer NOT NULL
);
CREATE INDEX notification_newsletter_history_profile ON notification_newsletter_history (user_id, profile_id, sent_at);
CREATE INDEX notification_newsletter_history_retention ON notification_newsletter_history (sent_at);
-- A retained, re-encrypted snapshot survives removal of the plugin installation.
CREATE TABLE notification_newsletter_imports (
    installation_id integer PRIMARY KEY,
    snapshot text NOT NULL,
    result jsonb NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE notification_newsletter_imports;
DROP TABLE notification_newsletter_history;
DROP TABLE notification_newsletter_settings;
DROP TABLE notification_delivery_preferences;
