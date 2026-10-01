-- +goose Up
CREATE TABLE partner_balance (
 id INTEGER PRIMARY KEY,
 partner_id BIGINT NOT NULL REFERENCES partner(id),
 direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),
 amount_cents BIGINT NOT NULL DEFAULT 0 CHECK(typeof(amount_cents)='integer'),
 entry_count BIGINT NOT NULL DEFAULT 0 CHECK(entry_count >= 0),
 update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(partner_id,direction)
);
CREATE TABLE partner_balance_entry (
 id INTEGER PRIMARY KEY,
 partner_id BIGINT NOT NULL REFERENCES partner(id),
 direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),
 entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('OPENING','REVERSAL')),
 amount_cents BIGINT NOT NULL CHECK(amount_cents <> 0 AND typeof(amount_cents)='integer'),
 balance_before_cents BIGINT NOT NULL CHECK(typeof(balance_before_cents)='integer'),
 balance_after_cents BIGINT NOT NULL CHECK(typeof(balance_after_cents)='integer'),
 business_date DATE NOT NULL,
 effective_at DATETIME NOT NULL,
 description VARCHAR(500) NOT NULL,
 document_no VARCHAR(40) NOT NULL UNIQUE,
 operator_id BIGINT NOT NULL REFERENCES sys_user(id),
 request_key VARCHAR(100) UNIQUE,
 reverses_id BIGINT REFERENCES partner_balance_entry(id),
 reversed_by_id BIGINT REFERENCES partner_balance_entry(id),
 create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK(balance_after_cents=balance_before_cents+amount_cents),
 CHECK((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<0 AND reverses_id IS NOT NULL))
);
CREATE UNIQUE INDEX uk_partner_balance_reversal ON partner_balance_entry(reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX idx_partner_balance_entry_scope ON partner_balance_entry(partner_id,direction,effective_at,id);
-- +goose Down
DROP TABLE IF EXISTS partner_balance_entry;
DROP TABLE IF EXISTS partner_balance;
