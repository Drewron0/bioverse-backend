-- name: GetAppSetting :one
SELECT setting_key, setting_value, description, updated_at
FROM app_settings
WHERE setting_key = $1
LIMIT 1;

-- name: UpsertAppSetting :one
INSERT INTO app_settings (setting_key, setting_value, description, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (setting_key) DO UPDATE
SET setting_value = EXCLUDED.setting_value,
    description = EXCLUDED.description,
    updated_at = NOW()
RETURNING *;

-- name: RecordWebhookEvent :one
INSERT INTO webhook_events (
    id, provider, external_event_id, event_type, payload, status
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (provider, external_event_id) DO NOTHING
RETURNING *;

-- name: GetWebhookEventByProviderAndExternalID :one
SELECT * FROM webhook_events
WHERE provider = $1 AND external_event_id = $2
LIMIT 1;

-- name: UpdateWebhookEventStatus :one
UPDATE webhook_events
SET status = $3,
    error_message = $4,
    processed_at = $5
WHERE provider = $1 AND external_event_id = $2
RETURNING *;
