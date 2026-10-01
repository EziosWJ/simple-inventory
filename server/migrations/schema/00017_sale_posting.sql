-- +goose Up
ALTER TABLE sale_document DROP CONSTRAINT sale_document_status_check;
ALTER TABLE sale_document ADD COLUMN partner_name VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE sale_document ADD COLUMN owner_name VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE sale_document ADD COLUMN owner_phone VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE sale_document ADD COLUMN owner_address VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE sale_document ADD COLUMN posted_by BIGINT REFERENCES sys_user(id);
ALTER TABLE sale_document ADD COLUMN posted_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE sale_document ADD CONSTRAINT sale_document_status_check CHECK(status IN ('DRAFT','POSTED','CANCELLED'));
ALTER TABLE sale_document DROP CONSTRAINT sale_document_check;
ALTER TABLE sale_document ADD CONSTRAINT sale_document_state_check CHECK (
 (status='DRAFT' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL AND posted_by IS NULL AND posted_at IS NULL) OR
 (status='POSTED' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL AND posted_by IS NOT NULL AND posted_at IS NOT NULL) OR
 (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0 AND ((posted_by IS NULL AND posted_at IS NULL) OR (posted_by IS NOT NULL AND posted_at IS NOT NULL)))
);
ALTER TABLE inventory_entry ADD COLUMN sale_id BIGINT REFERENCES sale_document(id);
ALTER TABLE inventory_entry ADD COLUMN sale_item_id BIGINT REFERENCES sale_document_item(id);
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL','SALE','SALE_CANCEL'));
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_source_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_source_check CHECK (
 (adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL AND sale_id IS NULL AND sale_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NOT NULL AND sale_item_id IS NOT NULL)
);
CREATE UNIQUE INDEX uk_inventory_entry_sale_item ON inventory_entry(sale_item_id,entry_type) WHERE sale_item_id IS NOT NULL;
CREATE INDEX idx_inventory_entry_sale ON inventory_entry(sale_id,id);
ALTER TABLE partner_balance_entry ADD COLUMN sale_id BIGINT REFERENCES sale_document(id);
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL) OR
 (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
CREATE UNIQUE INDEX uk_partner_balance_sale ON partner_balance_entry(sale_id) WHERE sale_id IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS uk_partner_balance_sale;
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
ALTER TABLE partner_balance_entry DROP COLUMN sale_id;
DROP INDEX IF EXISTS idx_inventory_entry_sale;
DROP INDEX IF EXISTS uk_inventory_entry_sale_item;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_source_check;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL'));
ALTER TABLE inventory_entry DROP COLUMN sale_item_id;
ALTER TABLE inventory_entry DROP COLUMN sale_id;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_source_check CHECK ((adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL));
ALTER TABLE sale_document DROP CONSTRAINT sale_document_state_check;
ALTER TABLE sale_document DROP COLUMN posted_at;
ALTER TABLE sale_document DROP COLUMN posted_by;
ALTER TABLE sale_document DROP COLUMN owner_address;
ALTER TABLE sale_document DROP COLUMN owner_phone;
ALTER TABLE sale_document DROP COLUMN owner_name;
ALTER TABLE sale_document DROP COLUMN partner_name;
ALTER TABLE sale_document ADD CONSTRAINT sale_document_check CHECK((status='DRAFT' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0));
