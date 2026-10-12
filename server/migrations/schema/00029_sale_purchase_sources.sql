-- +goose Up
CREATE TABLE sale_purchase_source (
 id BIGSERIAL PRIMARY KEY,
 sale_item_id BIGINT NOT NULL REFERENCES sale_document_item(id) ON DELETE CASCADE,
 purchase_item_id BIGINT NOT NULL REFERENCES purchase_document_item(id),
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli>0),
 confirm_later_date BOOLEAN NOT NULL DEFAULT FALSE,
 UNIQUE(sale_item_id,purchase_item_id)
);
CREATE INDEX sale_purchase_source_purchase_idx ON sale_purchase_source(purchase_item_id);
CREATE TABLE sale_purchase_source_history (
 id BIGSERIAL PRIMARY KEY, sale_id BIGINT NOT NULL REFERENCES sale_document(id),
 sale_item_id BIGINT NOT NULL,
 before_data TEXT NOT NULL, after_data TEXT NOT NULL,
 operator_id BIGINT NOT NULL REFERENCES sys_user(id),
 create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX sale_purchase_source_history_document_idx ON sale_purchase_source_history(sale_id,id);
-- +goose Down
DROP TABLE sale_purchase_source_history;
DROP TABLE sale_purchase_source;
