-- +goose Up
CREATE TABLE sale_save_request (
 actor_id INTEGER NOT NULL REFERENCES sys_user(id),
 operation VARCHAR(10) NOT NULL CHECK(operation IN ('CREATE','EDIT')),
 request_key VARCHAR(100) NOT NULL CHECK(length(request_key)<=100),
 fingerprint VARCHAR(64) NOT NULL,
 document_id INTEGER REFERENCES sale_document(id),
 saved_version INTEGER,
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(actor_id,operation,request_key),
 CHECK((document_id IS NULL AND saved_version IS NULL) OR (document_id IS NOT NULL AND saved_version IS NOT NULL AND saved_version>0))
);
-- +goose Down
DROP TABLE sale_save_request;
