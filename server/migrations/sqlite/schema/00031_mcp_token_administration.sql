-- +goose Up
-- SQLite cannot drop a NOT NULL constraint in place, so the table is rebuilt
-- to make expires_at nullable while existing rows keep their values.
CREATE TABLE mcp_token_new (
 id INTEGER PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES sys_user(id),
 client_id VARCHAR(100) NOT NULL,
 digest VARCHAR(64) NOT NULL UNIQUE,
 scopes TEXT NOT NULL,
 expires_at DATETIME,
 disabled_at DATETIME,
 secret_version INTEGER,
 secret_cipher TEXT,
 revoked_at DATETIME,
 created_at DATETIME NOT NULL,
 created_by VARCHAR(100) NOT NULL,
 revoked_by VARCHAR(100)
);
INSERT INTO mcp_token_new(id,user_id,client_id,digest,scopes,expires_at,disabled_at,secret_version,secret_cipher,revoked_at,created_at,created_by,revoked_by)
 SELECT id,user_id,client_id,digest,scopes,expires_at,NULL,NULL,NULL,revoked_at,created_at,created_by,revoked_by FROM mcp_token;
DROP TABLE mcp_token;
ALTER TABLE mcp_token_new RENAME TO mcp_token;
CREATE INDEX mcp_token_user_idx ON mcp_token(user_id);
-- +goose Down
-- SQLite table rebuilds are not reversible; the original NOT NULL constraint
-- cannot be restored without discarding permanent tokens.
SELECT 1;
