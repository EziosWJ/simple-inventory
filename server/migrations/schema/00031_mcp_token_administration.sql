-- +goose Up
-- Administration adds a display name (reusing client_id), an independent
-- disabled state, an optional recoverable secret and a permanent-validity
-- representation. Authentication keeps using digest only.
ALTER TABLE mcp_token ADD COLUMN disabled_at TIMESTAMPTZ;
ALTER TABLE mcp_token ADD COLUMN secret_version INTEGER;
ALTER TABLE mcp_token ADD COLUMN secret_cipher TEXT;
ALTER TABLE mcp_token ALTER COLUMN expires_at DROP NOT NULL;
-- +goose Down
-- A permanent token has no expiry to restore, so the downgrade only restores
-- NOT NULL while every remaining row still carries an expiry.
ALTER TABLE mcp_token ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE mcp_token DROP COLUMN secret_cipher;
ALTER TABLE mcp_token DROP COLUMN secret_version;
ALTER TABLE mcp_token DROP COLUMN disabled_at;
