-- +goose Up
ALTER TABLE purchase_document DROP CONSTRAINT purchase_document_status_check;
ALTER TABLE purchase_document ADD COLUMN posted_by BIGINT REFERENCES sys_user(id);
ALTER TABLE purchase_document ADD COLUMN posted_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE purchase_document ADD CONSTRAINT purchase_document_status_check CHECK(status IN ('DRAFT','POSTED','CANCELLED'));
ALTER TABLE purchase_document DROP CONSTRAINT purchase_document_check;
ALTER TABLE purchase_document ADD CONSTRAINT purchase_document_state_check CHECK (
 (status='DRAFT' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL AND posted_by IS NULL AND posted_at IS NULL) OR
 (status='POSTED' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL AND posted_by IS NOT NULL AND posted_at IS NOT NULL) OR
 (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0 AND ((posted_by IS NULL AND posted_at IS NULL) OR (posted_by IS NOT NULL AND posted_at IS NOT NULL)))
);
ALTER TABLE inventory_entry ALTER COLUMN adjustment_id DROP NOT NULL;
ALTER TABLE inventory_entry ALTER COLUMN adjustment_item_id DROP NOT NULL;
ALTER TABLE inventory_entry ADD COLUMN purchase_id BIGINT REFERENCES purchase_document(id);
ALTER TABLE inventory_entry ADD COLUMN purchase_item_id BIGINT REFERENCES purchase_document_item(id);
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL'));
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_source_check CHECK ((adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL) OR (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL));
CREATE UNIQUE INDEX uk_inventory_entry_purchase_item ON inventory_entry(purchase_item_id,entry_type) WHERE purchase_item_id IS NOT NULL;
CREATE INDEX idx_inventory_entry_purchase ON inventory_entry(purchase_id,id);
ALTER TABLE partner_balance_entry ADD COLUMN purchase_id BIGINT REFERENCES purchase_document(id);
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
CREATE UNIQUE INDEX uk_partner_balance_purchase ON partner_balance_entry(purchase_id) WHERE purchase_id IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS uk_partner_balance_purchase;
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK ((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL));
ALTER TABLE partner_balance_entry DROP COLUMN purchase_id;
DROP INDEX IF EXISTS idx_inventory_entry_purchase;
DROP INDEX IF EXISTS uk_inventory_entry_purchase_item;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_source_check;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER'));
ALTER TABLE inventory_entry DROP COLUMN purchase_item_id;
ALTER TABLE inventory_entry DROP COLUMN purchase_id;
ALTER TABLE inventory_entry ALTER COLUMN adjustment_item_id SET NOT NULL;
ALTER TABLE inventory_entry ALTER COLUMN adjustment_id SET NOT NULL;
ALTER TABLE purchase_document DROP CONSTRAINT purchase_document_state_check;
ALTER TABLE purchase_document DROP COLUMN posted_at;
ALTER TABLE purchase_document DROP COLUMN posted_by;
ALTER TABLE purchase_document ADD CONSTRAINT purchase_document_check CHECK((status='DRAFT' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL) OR (status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND length(trim(cancel_reason))>0));
