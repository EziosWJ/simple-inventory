-- +goose Up
CREATE TABLE inventory_adjustment (
 id INTEGER PRIMARY KEY,
 document_no VARCHAR(50) NOT NULL UNIQUE,
 status VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','POSTED','CANCELLED')),
 version BIGINT NOT NULL DEFAULT 1 CHECK(version >= 1),
 created_by BIGINT NOT NULL REFERENCES sys_user(id),
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_inventory_adjustment_created ON inventory_adjustment(create_time,id);
CREATE INDEX idx_inventory_adjustment_status ON inventory_adjustment(status,id);
CREATE TABLE inventory_adjustment_item (
 id INTEGER PRIMARY KEY,
 adjustment_id BIGINT NOT NULL REFERENCES inventory_adjustment(id),
 product_id BIGINT NOT NULL REFERENCES product(id),
 product_type VARCHAR(20) NOT NULL CHECK(product_type='GOODS'),
 unit VARCHAR(50) NOT NULL CHECK(length(trim(unit)) > 0),
 quantity_milli BIGINT NOT NULL CHECK(quantity_milli <> 0 AND typeof(quantity_milli)='integer'),
 reason VARCHAR(20) NOT NULL CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER')),
 remark VARCHAR(500),
 UNIQUE(adjustment_id,product_id),
 CHECK((reason IN ('OPENING','SURPLUS') AND quantity_milli > 0) OR (reason IN ('SHORTAGE','DAMAGE') AND quantity_milli < 0) OR (reason='OTHER' AND remark IS NOT NULL AND length(trim(remark)) > 0))
);
CREATE INDEX idx_inventory_adjustment_item_product ON inventory_adjustment_item(product_id,adjustment_id);
-- +goose Down
DROP TABLE IF EXISTS inventory_adjustment_item;
DROP TABLE IF EXISTS inventory_adjustment;
