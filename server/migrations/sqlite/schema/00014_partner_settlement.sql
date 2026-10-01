-- +goose Up
CREATE TABLE partner_balance_entry_new (
 id INTEGER PRIMARY KEY,
 partner_id BIGINT NOT NULL REFERENCES partner(id),
 direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),
 entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('OPENING','RECEIPT','PAYMENT','REVERSAL')),
 amount_cents BIGINT NOT NULL CHECK(amount_cents<>0 AND typeof(amount_cents)='integer'),
 balance_before_cents BIGINT NOT NULL CHECK(typeof(balance_before_cents)='integer'),
 balance_after_cents BIGINT NOT NULL CHECK(typeof(balance_after_cents)='integer'),
 business_date DATE NOT NULL,effective_at DATETIME NOT NULL,description VARCHAR(500) NOT NULL,
 document_no VARCHAR(40) NOT NULL UNIQUE,operator_id BIGINT NOT NULL REFERENCES sys_user(id),
 request_key VARCHAR(100) UNIQUE,reverses_id BIGINT REFERENCES partner_balance_entry_new(id),
 reversed_by_id BIGINT REFERENCES partner_balance_entry_new(id),create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 payment_method VARCHAR(20) NOT NULL DEFAULT '',transaction_no VARCHAR(100) NOT NULL DEFAULT '',
 CHECK(balance_after_cents=balance_before_cents+amount_cents),
 CHECK((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL))
);
INSERT INTO partner_balance_entry_new(id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time)
SELECT id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time FROM partner_balance_entry;
DROP TABLE partner_balance_entry;
ALTER TABLE partner_balance_entry_new RENAME TO partner_balance_entry;
CREATE UNIQUE INDEX uk_partner_balance_reversal ON partner_balance_entry(reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX idx_partner_balance_entry_scope ON partner_balance_entry(partner_id,direction,effective_at,id);
-- +goose Down
CREATE TABLE partner_balance_entry_old (
 id INTEGER PRIMARY KEY,partner_id BIGINT NOT NULL REFERENCES partner(id),direction VARCHAR(20) NOT NULL CHECK(direction IN ('CUSTOMER','SUPPLIER')),
 entry_type VARCHAR(20) NOT NULL CHECK(entry_type IN ('OPENING','REVERSAL')),amount_cents BIGINT NOT NULL CHECK(amount_cents<>0 AND typeof(amount_cents)='integer'),
 balance_before_cents BIGINT NOT NULL CHECK(typeof(balance_before_cents)='integer'),balance_after_cents BIGINT NOT NULL CHECK(typeof(balance_after_cents)='integer'),business_date DATE NOT NULL,effective_at DATETIME NOT NULL,
 description VARCHAR(500) NOT NULL,document_no VARCHAR(40) NOT NULL UNIQUE,operator_id BIGINT NOT NULL REFERENCES sys_user(id),request_key VARCHAR(100) UNIQUE,
 reverses_id BIGINT REFERENCES partner_balance_entry_old(id),reversed_by_id BIGINT REFERENCES partner_balance_entry_old(id),create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK(balance_after_cents=balance_before_cents+amount_cents),CHECK((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<0 AND reverses_id IS NOT NULL))
);
INSERT INTO partner_balance_entry_old SELECT id,partner_id,direction,entry_type,amount_cents,balance_before_cents,balance_after_cents,business_date,effective_at,description,document_no,operator_id,request_key,reverses_id,reversed_by_id,create_time FROM partner_balance_entry WHERE entry_type IN ('OPENING','REVERSAL');
DROP TABLE partner_balance_entry;
ALTER TABLE partner_balance_entry_old RENAME TO partner_balance_entry;
CREATE UNIQUE INDEX uk_partner_balance_reversal ON partner_balance_entry(reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX idx_partner_balance_entry_scope ON partner_balance_entry(partner_id,direction,effective_at,id);
