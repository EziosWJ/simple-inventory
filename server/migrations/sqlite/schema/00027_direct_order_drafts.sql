-- +goose Up
CREATE TABLE direct_order_document (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 document_no VARCHAR(50) NOT NULL UNIQUE,
 supplier_id INTEGER NOT NULL REFERENCES partner(id),
 customer_id INTEGER NOT NULL REFERENCES partner(id),
 business_date DATE NOT NULL,
 status VARCHAR(10) NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','POSTED','CANCELLED')),
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
 delivery_contact VARCHAR(100), delivery_phone VARCHAR(50), delivery_address VARCHAR(500), remark VARCHAR(500),
 created_by INTEGER NOT NULL REFERENCES sys_user(id),
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX direct_order_business_date_idx ON direct_order_document(business_date,id);
CREATE TABLE direct_order_item (
 id INTEGER PRIMARY KEY AUTOINCREMENT, document_id INTEGER NOT NULL REFERENCES direct_order_document(id),
 product_id INTEGER NOT NULL REFERENCES product(id),
 product_type VARCHAR(10) NOT NULL CHECK(product_type IN ('GOODS','SERVICE')),
 unit VARCHAR(50) NOT NULL,
 quantity_milli INTEGER NOT NULL CHECK(quantity_milli>0),
 purchase_price_cents INTEGER NOT NULL CHECK(purchase_price_cents>=0),
 sale_price_cents INTEGER NOT NULL CHECK(sale_price_cents>=0),
 purchase_amount_cents INTEGER NOT NULL CHECK(purchase_amount_cents>=0),
 sale_amount_cents INTEGER NOT NULL CHECK(sale_amount_cents>=0),
 remark VARCHAR(500),
 CHECK(product_type='GOODS' OR (purchase_price_cents=0 AND purchase_amount_cents=0))
);
CREATE INDEX direct_order_item_document_idx ON direct_order_item(document_id,id);
CREATE TABLE direct_order_save_request (
 actor_id INTEGER NOT NULL REFERENCES sys_user(id),
 operation VARCHAR(10) NOT NULL CHECK(operation IN ('CREATE','EDIT')),
 request_key VARCHAR(100) NOT NULL CHECK(length(request_key)<=100),
 fingerprint VARCHAR(64) NOT NULL,
 document_id INTEGER REFERENCES direct_order_document(id), saved_version INTEGER,
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(actor_id,operation,request_key),
 CHECK((document_id IS NULL AND saved_version IS NULL) OR (document_id IS NOT NULL AND saved_version IS NOT NULL AND saved_version>0))
);
-- +goose Down
DROP TABLE direct_order_save_request;
DROP TABLE direct_order_item;
DROP TABLE direct_order_document;
