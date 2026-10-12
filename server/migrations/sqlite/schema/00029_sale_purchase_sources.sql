-- +goose Up
CREATE TABLE sale_purchase_source (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 sale_item_id INTEGER NOT NULL REFERENCES sale_document_item(id) ON DELETE CASCADE,
 purchase_item_id INTEGER NOT NULL REFERENCES purchase_document_item(id),
 quantity_milli INTEGER NOT NULL CHECK(quantity_milli>0),
 confirm_later_date INTEGER NOT NULL DEFAULT FALSE,
 UNIQUE(sale_item_id,purchase_item_id)
);
CREATE INDEX sale_purchase_source_purchase_idx ON sale_purchase_source(purchase_item_id);
CREATE TABLE sale_purchase_source_history (
 id INTEGER PRIMARY KEY AUTOINCREMENT, sale_id INTEGER NOT NULL REFERENCES sale_document(id),
 sale_item_id INTEGER NOT NULL,
 before_data TEXT NOT NULL, after_data TEXT NOT NULL,
 operator_id INTEGER NOT NULL REFERENCES sys_user(id),
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX sale_purchase_source_history_document_idx ON sale_purchase_source_history(sale_id,id);
-- +goose Down
DROP TABLE sale_purchase_source_history;
DROP TABLE sale_purchase_source;
