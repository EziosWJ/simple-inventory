-- +goose Up
CREATE TABLE direct_order_document (
 id BIGSERIAL PRIMARY KEY,
 document_no VARCHAR(50) NOT NULL UNIQUE,
 supplier_id BIGINT NOT NULL REFERENCES partner(id),
 customer_id BIGINT NOT NULL REFERENCES partner(id),
 business_date DATE NOT NULL,
 status VARCHAR(10) NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','POSTED','CANCELLED')),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 delivery_contact VARCHAR(100), delivery_phone VARCHAR(50), delivery_address VARCHAR(500), remark VARCHAR(500),
 created_by BIGINT NOT NULL REFERENCES sys_user(id),
 create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX direct_order_business_date_idx ON direct_order_document(business_date,id);
CREATE TABLE direct_order_item (
 id BIGSERIAL PRIMARY KEY, document_id BIGINT NOT NULL REFERENCES direct_order_document(id),
 product_id BIGINT NOT NULL REFERENCES product(id),
 product_type VARCHAR(10) NOT NULL CHECK(product_type IN ('GOODS','SERVICE')),
 unit VARCHAR(50) NOT NULL,
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli>0),
 purchase_price_cents BIGINT NOT NULL CHECK(purchase_price_cents>=0),
 sale_price_cents BIGINT NOT NULL CHECK(sale_price_cents>=0),
 purchase_amount_cents BIGINT NOT NULL CHECK(purchase_amount_cents>=0),
 sale_amount_cents BIGINT NOT NULL CHECK(sale_amount_cents>=0),
 remark VARCHAR(500),
 CHECK(product_type='GOODS' OR (purchase_price_cents=0 AND purchase_amount_cents=0))
);
CREATE INDEX direct_order_item_document_idx ON direct_order_item(document_id,id);
CREATE TABLE direct_order_save_request (
 actor_id BIGINT NOT NULL REFERENCES sys_user(id),
 operation VARCHAR(10) NOT NULL CHECK(operation IN ('CREATE','EDIT')),
 request_key VARCHAR(100) NOT NULL CHECK(length(request_key)<=100),
 fingerprint VARCHAR(64) NOT NULL,
 document_id BIGINT REFERENCES direct_order_document(id), saved_version BIGINT,
 create_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(actor_id,operation,request_key),
 CHECK((document_id IS NULL AND saved_version IS NULL) OR (document_id IS NOT NULL AND saved_version IS NOT NULL AND saved_version>0))
);
-- +goose Down
DROP TABLE direct_order_save_request;
DROP TABLE direct_order_item;
DROP TABLE direct_order_document;
