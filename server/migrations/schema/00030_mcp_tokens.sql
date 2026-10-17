-- +goose Up
CREATE TABLE mcp_token (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES sys_user(id),
 client_id VARCHAR(100) NOT NULL,
 digest VARCHAR(64) NOT NULL UNIQUE,
 scopes TEXT NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL,
 created_by VARCHAR(100) NOT NULL,
 revoked_by VARCHAR(100)
);
CREATE INDEX mcp_token_user_idx ON mcp_token(user_id);
-- +goose Down
DROP TABLE mcp_token;
