-- +goose Up
ALTER TABLE purchase_return_document ADD COLUMN posted_by BIGINT REFERENCES sys_user(id);
ALTER TABLE purchase_return_document ADD COLUMN posted_at DATETIME;
CREATE TRIGGER purchase_return_state_guard BEFORE UPDATE ON purchase_return_document
WHEN NOT ((NEW.status='DRAFT' AND NEW.posted_by IS NULL AND NEW.posted_at IS NULL AND NEW.cancelled_by IS NULL AND NEW.cancelled_at IS NULL AND NEW.cancel_reason IS NULL) OR
 (NEW.status='POSTED' AND NEW.posted_by IS NOT NULL AND NEW.posted_at IS NOT NULL AND NEW.cancelled_by IS NULL AND NEW.cancelled_at IS NULL AND NEW.cancel_reason IS NULL) OR
 (NEW.status='CANCELLED' AND NEW.cancelled_by IS NOT NULL AND NEW.cancelled_at IS NOT NULL AND length(trim(NEW.cancel_reason))>0 AND ((NEW.posted_by IS NULL AND NEW.posted_at IS NULL) OR (NEW.posted_by IS NOT NULL AND NEW.posted_at IS NOT NULL))))
BEGIN SELECT RAISE(ABORT,'invalid purchase return state'); END;
CREATE INDEX idx_purchase_return_posted ON purchase_return_document(purchase_id,posted_at,id);
CREATE TABLE inventory_entry_new (
 id INTEGER PRIMARY KEY,product_id BIGINT NOT NULL REFERENCES product(id),adjustment_id BIGINT REFERENCES inventory_adjustment(id),adjustment_item_id BIGINT REFERENCES inventory_adjustment_item(id),purchase_id BIGINT REFERENCES purchase_document(id),purchase_item_id BIGINT REFERENCES purchase_document_item(id),sale_id BIGINT REFERENCES sale_document(id),sale_item_id BIGINT REFERENCES sale_document_item(id),purchase_return_id BIGINT REFERENCES purchase_return_document(id),purchase_return_item_id BIGINT REFERENCES purchase_return_document_item(id),entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('ORIGINAL','REVERSAL')),
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli<>0 AND typeof(quantity_milli)='integer'),balance_before_milli BIGINT NOT NULL CHECK(balance_before_milli>=0 AND typeof(balance_before_milli)='integer'),balance_after_milli BIGINT NOT NULL CHECK(balance_after_milli>=0 AND typeof(balance_after_milli)='integer'),
 reason VARCHAR(30) NOT NULL CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL','SALE','SALE_CANCEL','PURCHASE_RETURN','PURCHASE_RETURN_CANCEL')),remark VARCHAR(500),product_code VARCHAR(50) NOT NULL,product_name VARCHAR(200) NOT NULL,product_model VARCHAR(100),product_specification VARCHAR(200),unit VARCHAR(50) NOT NULL CHECK(length(trim(unit))>0),operator_id BIGINT NOT NULL REFERENCES sys_user(id),occurred_at DATETIME NOT NULL,create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(adjustment_item_id,entry_type),UNIQUE(purchase_item_id,entry_type),UNIQUE(sale_item_id,entry_type),UNIQUE(purchase_return_item_id,entry_type),
 CHECK((adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NOT NULL AND sale_item_id IS NOT NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NOT NULL AND purchase_return_item_id IS NOT NULL))
);
INSERT INTO inventory_entry_new(id,product_id,adjustment_id,adjustment_item_id,purchase_id,purchase_item_id,sale_id,sale_item_id,entry_type,quantity_milli,balance_before_milli,balance_after_milli,reason,remark,product_code,product_name,product_model,product_specification,unit,operator_id,occurred_at,create_time)
 SELECT id,product_id,adjustment_id,adjustment_item_id,purchase_id,purchase_item_id,sale_id,sale_item_id,entry_type,quantity_milli,balance_before_milli,balance_after_milli,reason,remark,product_code,product_name,product_model,product_specification,unit,operator_id,occurred_at,create_time FROM inventory_entry;
CREATE TABLE partner_balance_entry_new (
 id INTEGER PRIMARY KEY,partner_id BIGINT NOT NULL REFERENCES partner(id),direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),entry_type VARCHAR(30) NOT NULL CHECK(entry_type IN ('OPENING','RECEIPT','PAYMENT','PURCHASE','SALE','PURCHASE_RETURN','REVERSAL')),
 amount_cents BIGINT NOT NULL CHECK(amount_cents<>0 AND typeof(amount_cents)='integer'),balance_before_cents BIGINT NOT NULL CHECK(typeof(balance_before_cents)='integer'),balance_after_cents BIGINT NOT NULL CHECK(typeof(balance_after_cents)='integer'),business_date DATE NOT NULL,effective_at DATETIME NOT NULL,description VARCHAR(500) NOT NULL,document_no VARCHAR(40) NOT NULL UNIQUE,operator_id BIGINT NOT NULL REFERENCES sys_user(id),request_key VARCHAR(100) UNIQUE,reverses_id BIGINT REFERENCES partner_balance_entry_new(id),reversed_by_id BIGINT REFERENCES partner_balance_entry_new(id),create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,payment_method VARCHAR(20) NOT NULL DEFAULT '',transaction_no VARCHAR(100) NOT NULL DEFAULT '',purchase_id BIGINT REFERENCES purchase_document(id),sale_id BIGINT REFERENCES sale_document(id),purchase_return_id BIGINT REFERENCES purchase_return_document(id),
 CHECK(balance_after_cents=balance_before_cents+amount_cents),
 CHECK((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL AND purchase_return_id IS NULL) OR (entry_type='PURCHASE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NOT NULL) OR (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL))
);
INSERT INTO partner_balance_entry_new(id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time,payment_method,transaction_no,purchase_id,sale_id)
 SELECT id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time,payment_method,transaction_no,purchase_id,sale_id FROM partner_balance_entry;
DROP TABLE inventory_entry;
DROP TABLE partner_balance_entry;
ALTER TABLE inventory_entry_new RENAME TO inventory_entry;
ALTER TABLE partner_balance_entry_new RENAME TO partner_balance_entry;
CREATE INDEX idx_inventory_entry_product ON inventory_entry(product_id,occurred_at,id);
CREATE INDEX idx_inventory_entry_occurred ON inventory_entry(occurred_at,id);
CREATE INDEX idx_inventory_entry_adjustment ON inventory_entry(adjustment_id,id);
CREATE INDEX idx_inventory_entry_purchase ON inventory_entry(purchase_id,id);
CREATE INDEX idx_inventory_entry_sale ON inventory_entry(sale_id,id);
CREATE INDEX idx_inventory_entry_purchase_return ON inventory_entry(purchase_return_id,id);
CREATE UNIQUE INDEX uk_partner_balance_reversal ON partner_balance_entry(reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX idx_partner_balance_entry_scope ON partner_balance_entry(partner_id,direction,effective_at,id);
CREATE UNIQUE INDEX uk_partner_balance_purchase ON partner_balance_entry(purchase_id) WHERE purchase_id IS NOT NULL;
CREATE UNIQUE INDEX uk_partner_balance_sale ON partner_balance_entry(sale_id) WHERE sale_id IS NOT NULL;
CREATE UNIQUE INDEX uk_partner_balance_purchase_return ON partner_balance_entry(purchase_return_id) WHERE purchase_return_id IS NOT NULL;
-- +goose Down
DROP TRIGGER IF EXISTS purchase_return_state_guard;
SELECT 1;
