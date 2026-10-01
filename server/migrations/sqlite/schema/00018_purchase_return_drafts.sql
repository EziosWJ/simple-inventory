-- +goose Up
CREATE TABLE purchase_return_document (
 id INTEGER PRIMARY KEY AUTOINCREMENT, document_no VARCHAR(40) NOT NULL UNIQUE,
 purchase_id INTEGER NOT NULL REFERENCES purchase_document(id), partner_id INTEGER NOT NULL REFERENCES partner(id), business_date DATE NOT NULL,
 status VARCHAR(20) NOT NULL CHECK(status IN ('DRAFT','POSTED','CANCELLED')), version INTEGER NOT NULL CHECK(version>0), remark VARCHAR(500),
 created_by INTEGER NOT NULL REFERENCES sys_user(id), create_time DATETIME NOT NULL, cancelled_by INTEGER REFERENCES sys_user(id), cancelled_at DATETIME, cancel_reason VARCHAR(500),
 CHECK((status IN ('DRAFT','POSTED') AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0))
);
CREATE INDEX idx_purchase_return_origin ON purchase_return_document(purchase_id,id);
CREATE INDEX idx_purchase_return_partner_date ON purchase_return_document(partner_id,business_date,id);
CREATE INDEX idx_purchase_return_status_id ON purchase_return_document(status,id);
CREATE TABLE purchase_return_document_item (
 id INTEGER PRIMARY KEY AUTOINCREMENT, document_id INTEGER NOT NULL REFERENCES purchase_return_document(id), purchase_item_id INTEGER NOT NULL REFERENCES purchase_document_item(id),
 product_id INTEGER NOT NULL REFERENCES product(id), product_code VARCHAR(50) NOT NULL, product_name VARCHAR(200) NOT NULL, product_model VARCHAR(100), product_specification VARCHAR(200),
 unit VARCHAR(50) NOT NULL CHECK(length(trim(unit))>0), quantity_milli INTEGER NOT NULL CHECK(quantity_milli>0 AND typeof(quantity_milli)='integer'), unit_price_cents INTEGER NOT NULL CHECK(unit_price_cents>=0), amount_cents INTEGER NOT NULL CHECK(amount_cents>=0), remark VARCHAR(500)
);
CREATE INDEX idx_purchase_return_item_origin ON purchase_return_document_item(purchase_item_id,document_id);
CREATE INDEX idx_purchase_return_item_document ON purchase_return_document_item(document_id,id);
-- +goose Down
DROP TABLE IF EXISTS purchase_return_document_item;
DROP TABLE IF EXISTS purchase_return_document;
