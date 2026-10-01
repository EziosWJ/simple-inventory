-- +goose Up
CREATE TABLE sale_return_document (
 id INTEGER PRIMARY KEY AUTOINCREMENT, document_no VARCHAR(40) NOT NULL UNIQUE,
 sale_id INTEGER NOT NULL REFERENCES sale_document(id), partner_id INTEGER NOT NULL REFERENCES partner(id), business_date DATE NOT NULL,
 status VARCHAR(20) NOT NULL CHECK(status IN ('DRAFT','POSTED','CANCELLED')), version INTEGER NOT NULL CHECK(version>0), remark VARCHAR(500),
 created_by INTEGER NOT NULL REFERENCES sys_user(id), create_time DATETIME NOT NULL, posted_by INTEGER REFERENCES sys_user(id), posted_at DATETIME,
 cancelled_by INTEGER REFERENCES sys_user(id), cancelled_at DATETIME, cancel_reason VARCHAR(500),
 CHECK((status='DRAFT' AND posted_by IS NULL AND posted_at IS NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR
 (status='POSTED' AND posted_by IS NOT NULL AND posted_at IS NOT NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR
 (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0 AND ((posted_by IS NULL AND posted_at IS NULL) OR (posted_by IS NOT NULL AND posted_at IS NOT NULL))))
);
CREATE TRIGGER sale_return_state_guard BEFORE UPDATE ON sale_return_document
WHEN NOT ((NEW.status='DRAFT' AND NEW.posted_by IS NULL AND NEW.posted_at IS NULL AND NEW.cancelled_by IS NULL AND NEW.cancelled_at IS NULL AND NEW.cancel_reason IS NULL) OR
 (NEW.status='POSTED' AND NEW.posted_by IS NOT NULL AND NEW.posted_at IS NOT NULL AND NEW.cancelled_by IS NULL AND NEW.cancelled_at IS NULL AND NEW.cancel_reason IS NULL) OR
 (NEW.status='CANCELLED' AND NEW.cancelled_by IS NOT NULL AND NEW.cancelled_at IS NOT NULL AND length(trim(NEW.cancel_reason))>0 AND ((NEW.posted_by IS NULL AND NEW.posted_at IS NULL) OR (NEW.posted_by IS NOT NULL AND NEW.posted_at IS NOT NULL))))
BEGIN SELECT RAISE(ABORT,'invalid sale return state'); END;
CREATE INDEX idx_sale_return_origin ON sale_return_document(sale_id,id);
CREATE INDEX idx_sale_return_partner_date ON sale_return_document(partner_id,business_date,id);
CREATE INDEX idx_sale_return_status_id ON sale_return_document(status,id);
CREATE INDEX idx_sale_return_posted ON sale_return_document(sale_id,posted_at,id);
CREATE TABLE sale_return_document_item (
 id INTEGER PRIMARY KEY AUTOINCREMENT, document_id INTEGER NOT NULL REFERENCES sale_return_document(id), sale_item_id INTEGER NOT NULL REFERENCES sale_document_item(id),
 product_id INTEGER NOT NULL REFERENCES product(id), product_code VARCHAR(50) NOT NULL, product_name VARCHAR(200) NOT NULL, product_model VARCHAR(100), product_specification VARCHAR(200),
 unit VARCHAR(50) NOT NULL CHECK(length(trim(unit))>0), quantity_milli INTEGER NOT NULL CHECK(quantity_milli>0 AND typeof(quantity_milli)='integer'),
 unit_price_cents INTEGER NOT NULL CHECK(unit_price_cents>=0 AND typeof(unit_price_cents)='integer'), amount_cents INTEGER NOT NULL CHECK(amount_cents>=0 AND typeof(amount_cents)='integer'), remark VARCHAR(500)
);
CREATE INDEX idx_sale_return_item_origin ON sale_return_document_item(sale_item_id,document_id);
CREATE INDEX idx_sale_return_item_document ON sale_return_document_item(document_id,id);
-- +goose Down
DROP TRIGGER IF EXISTS sale_return_state_guard;
DROP TABLE IF EXISTS sale_return_document_item;
DROP TABLE IF EXISTS sale_return_document;
