-- +goose Up
CREATE TABLE purchase_save_request (
 actor_id BIGINT NOT NULL REFERENCES sys_user(id),
 operation VARCHAR(10) NOT NULL CHECK(operation IN ('CREATE','EDIT')),
 request_key VARCHAR(100) NOT NULL,
 fingerprint VARCHAR(64) NOT NULL,
 document_id BIGINT REFERENCES purchase_document(id),
 saved_version BIGINT,
 create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(actor_id,operation,request_key),
 CHECK((document_id IS NULL AND saved_version IS NULL) OR (document_id IS NOT NULL AND saved_version IS NOT NULL AND saved_version>0))
);
-- +goose Down
DROP TABLE purchase_save_request;
