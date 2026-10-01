-- +goose Up
CREATE TABLE sale_document_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,document_no VARCHAR(40) NOT NULL UNIQUE,partner_id INTEGER NOT NULL REFERENCES partner(id),partner_name VARCHAR(200) NOT NULL DEFAULT '',business_date DATE NOT NULL,
 status VARCHAR(20) NOT NULL CHECK(status IN ('DRAFT','POSTED','CANCELLED')),version INTEGER NOT NULL CHECK(version>0),delivery_contact VARCHAR(100),delivery_phone VARCHAR(50),delivery_address VARCHAR(500),
 owner_name VARCHAR(200) NOT NULL DEFAULT '',owner_phone VARCHAR(50) NOT NULL DEFAULT '',owner_address VARCHAR(500) NOT NULL DEFAULT '',remark VARCHAR(500),created_by INTEGER NOT NULL REFERENCES sys_user(id),create_time DATETIME NOT NULL,
 posted_by INTEGER REFERENCES sys_user(id),posted_at DATETIME,cancelled_by INTEGER REFERENCES sys_user(id),cancelled_at DATETIME,cancel_reason VARCHAR(500),
 CHECK((status='DRAFT' AND posted_by IS NULL AND posted_at IS NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR
 (status='POSTED' AND posted_by IS NOT NULL AND posted_at IS NOT NULL AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR
 (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0 AND ((posted_by IS NULL AND posted_at IS NULL) OR (posted_by IS NOT NULL AND posted_at IS NOT NULL))))
);
INSERT INTO sale_document_new(id,document_no,partner_id,business_date,status,version,delivery_contact,delivery_phone,delivery_address,remark,created_by,create_time,cancelled_by,cancelled_at,cancel_reason)
 SELECT id,document_no,partner_id,business_date,status,version,delivery_contact,delivery_phone,delivery_address,remark,created_by,create_time,cancelled_by,cancelled_at,cancel_reason FROM sale_document;
CREATE TABLE sale_document_item_new (
 id INTEGER PRIMARY KEY AUTOINCREMENT,document_id INTEGER NOT NULL REFERENCES sale_document_new(id),product_id INTEGER NOT NULL REFERENCES product(id),product_code VARCHAR(50) NOT NULL,product_name VARCHAR(200) NOT NULL,
 product_model VARCHAR(100),product_specification VARCHAR(200),product_type VARCHAR(20) NOT NULL CHECK(product_type IN ('GOODS','SERVICE')),unit VARCHAR(50) NOT NULL CHECK(length(trim(unit))>0),
 quantity_milli INTEGER NOT NULL CHECK(quantity_milli>0),unit_price_cents INTEGER NOT NULL CHECK(unit_price_cents>=0),amount_cents INTEGER NOT NULL CHECK(amount_cents>=0),remark VARCHAR(500)
);
INSERT INTO sale_document_item_new SELECT * FROM sale_document_item;
CREATE TABLE inventory_entry_new (
 id INTEGER PRIMARY KEY,product_id BIGINT NOT NULL REFERENCES product(id),adjustment_id BIGINT REFERENCES inventory_adjustment(id),adjustment_item_id BIGINT REFERENCES inventory_adjustment_item(id),purchase_id BIGINT REFERENCES purchase_document(id),purchase_item_id BIGINT REFERENCES purchase_document_item(id),sale_id BIGINT REFERENCES sale_document_new(id),sale_item_id BIGINT REFERENCES sale_document_item_new(id),entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('ORIGINAL','REVERSAL')),
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli<>0 AND typeof(quantity_milli)='integer'),balance_before_milli BIGINT NOT NULL CHECK(balance_before_milli>=0 AND typeof(balance_before_milli)='integer'),balance_after_milli BIGINT NOT NULL CHECK(balance_after_milli>=0 AND typeof(balance_after_milli)='integer'),
 reason VARCHAR(20) NOT NULL CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL','SALE','SALE_CANCEL')),remark VARCHAR(500),product_code VARCHAR(50) NOT NULL,product_name VARCHAR(200) NOT NULL,product_model VARCHAR(100),product_specification VARCHAR(200),unit VARCHAR(50) NOT NULL CHECK(length(trim(unit))>0),
 operator_id BIGINT NOT NULL REFERENCES sys_user(id),occurred_at DATETIME NOT NULL,create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(adjustment_item_id,entry_type),UNIQUE(purchase_item_id,entry_type),UNIQUE(sale_item_id,entry_type),
 CHECK((adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL AND sale_id IS NULL AND sale_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NOT NULL AND sale_item_id IS NOT NULL))
);
INSERT INTO inventory_entry_new(id,product_id,adjustment_id,adjustment_item_id,purchase_id,purchase_item_id,entry_type,quantity_milli,balance_before_milli,balance_after_milli,reason,remark,product_code,product_name,product_model,product_specification,unit,operator_id,occurred_at,create_time)
 SELECT id,product_id,adjustment_id,adjustment_item_id,purchase_id,purchase_item_id,entry_type,quantity_milli,balance_before_milli,balance_after_milli,reason,remark,product_code,product_name,product_model,product_specification,unit,operator_id,occurred_at,create_time FROM inventory_entry;
CREATE TABLE partner_balance_entry_new (
 id INTEGER PRIMARY KEY,partner_id BIGINT NOT NULL REFERENCES partner(id),direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('OPENING','RECEIPT','PAYMENT','PURCHASE','SALE','REVERSAL')),
 amount_cents BIGINT NOT NULL CHECK(amount_cents<>0 AND typeof(amount_cents)='integer'),balance_before_cents BIGINT NOT NULL CHECK(typeof(balance_before_cents)='integer'),balance_after_cents BIGINT NOT NULL CHECK(typeof(balance_after_cents)='integer'),business_date DATE NOT NULL,effective_at DATETIME NOT NULL,
 description VARCHAR(500) NOT NULL,document_no VARCHAR(40) NOT NULL UNIQUE,operator_id BIGINT NOT NULL REFERENCES sys_user(id),request_key VARCHAR(100) UNIQUE,reverses_id BIGINT REFERENCES partner_balance_entry_new(id),reversed_by_id BIGINT REFERENCES partner_balance_entry_new(id),create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 payment_method VARCHAR(20) NOT NULL DEFAULT '',transaction_no VARCHAR(100) NOT NULL DEFAULT '',purchase_id BIGINT REFERENCES purchase_document(id),sale_id BIGINT REFERENCES sale_document_new(id),
 CHECK(balance_after_cents=balance_before_cents+amount_cents),
 CHECK((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL) OR (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL) OR (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL) OR (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL) OR (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL))
);
INSERT INTO partner_balance_entry_new(id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time,payment_method,transaction_no,purchase_id)
 SELECT id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time,payment_method,transaction_no,purchase_id FROM partner_balance_entry;
DROP TABLE inventory_entry;
DROP TABLE partner_balance_entry;
DROP TABLE sale_document_item;
DROP TABLE sale_document;
ALTER TABLE sale_document_new RENAME TO sale_document;
ALTER TABLE sale_document_item_new RENAME TO sale_document_item;
ALTER TABLE inventory_entry_new RENAME TO inventory_entry;
ALTER TABLE partner_balance_entry_new RENAME TO partner_balance_entry;
CREATE INDEX idx_sale_document_partner_date ON sale_document(partner_id,business_date,id);
CREATE INDEX idx_sale_document_status_id ON sale_document(status,id);
CREATE INDEX idx_sale_document_item_product ON sale_document_item(product_id,document_id);
CREATE INDEX idx_inventory_entry_product ON inventory_entry(product_id,occurred_at,id);
CREATE INDEX idx_inventory_entry_occurred ON inventory_entry(occurred_at,id);
CREATE INDEX idx_inventory_entry_adjustment ON inventory_entry(adjustment_id,id);
CREATE INDEX idx_inventory_entry_purchase ON inventory_entry(purchase_id,id);
CREATE INDEX idx_inventory_entry_sale ON inventory_entry(sale_id,id);
CREATE UNIQUE INDEX uk_partner_balance_reversal ON partner_balance_entry(reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX idx_partner_balance_entry_scope ON partner_balance_entry(partner_id,direction,effective_at,id);
CREATE UNIQUE INDEX uk_partner_balance_purchase ON partner_balance_entry(purchase_id) WHERE purchase_id IS NOT NULL;
CREATE UNIQUE INDEX uk_partner_balance_sale ON partner_balance_entry(sale_id) WHERE sale_id IS NOT NULL;
-- +goose Down
SELECT 1;
