-- +goose Up
CREATE TABLE product (
 id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, type TEXT NOT NULL,
 brand TEXT, model TEXT, specification TEXT, category TEXT, unit TEXT NOT NULL,
 purchase_price_cents INTEGER, sale_price_cents INTEGER, remark TEXT,
 status INTEGER NOT NULL DEFAULT 1, create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK(type IN ('GOODS','SERVICE')), CHECK(status IN (0,1)),
 CHECK((purchase_price_cents IS NULL OR purchase_price_cents >= 0) AND (sale_price_cents IS NULL OR sale_price_cents >= 0))
);
CREATE INDEX idx_product_status_type_id ON product(status,type,id);
CREATE INDEX idx_product_category ON product(category);
CREATE TABLE partner (
 id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, type TEXT NOT NULL,
 is_customer INTEGER NOT NULL, is_supplier INTEGER NOT NULL,
 contact TEXT, phone TEXT, address TEXT, remark TEXT, invoice_name TEXT, tax_number TEXT,
 registered_address TEXT, registered_phone TEXT, bank_name TEXT, bank_account TEXT,
 status INTEGER NOT NULL DEFAULT 1, create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK(type IN ('COMPANY','PERSON')),
 CHECK(is_customer IN (0,1) AND is_supplier IN (0,1) AND (is_customer=1 OR is_supplier=1)),
 CHECK(status IN (0,1))
);
CREATE INDEX idx_partner_status_type_id ON partner(status,type,id);
CREATE TABLE warehouse (
 singleton_id INTEGER PRIMARY KEY CHECK(singleton_id=1), name TEXT NOT NULL, remark TEXT,
 update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose Down
DROP TABLE IF EXISTS warehouse;
DROP TABLE IF EXISTS partner;
DROP TABLE IF EXISTS product;
