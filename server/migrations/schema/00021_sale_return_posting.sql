-- +goose Up
-- Posting a sale return adds stock back and reduces the customer's receivable.
-- The document already carries the posted state from the draft migration, so
-- this migration only extends the shared ledgers with a sale-return source
-- alongside adjustment, purchase, sale and purchase return.
ALTER TABLE inventory_entry ADD COLUMN sale_return_id BIGINT REFERENCES sale_return_document(id);
ALTER TABLE inventory_entry ADD COLUMN sale_return_item_id BIGINT REFERENCES sale_return_document_item(id);
ALTER TABLE inventory_entry ALTER COLUMN reason TYPE VARCHAR(30);
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL','SALE','SALE_CANCEL','PURCHASE_RETURN','PURCHASE_RETURN_CANCEL','SALE_RETURN','SALE_RETURN_CANCEL'));
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_source_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_source_check CHECK (
 (adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL AND sale_return_id IS NULL AND sale_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL AND sale_return_id IS NULL AND sale_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NOT NULL AND sale_item_id IS NOT NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL AND sale_return_id IS NULL AND sale_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NOT NULL AND purchase_return_item_id IS NOT NULL AND sale_return_id IS NULL AND sale_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL AND sale_return_id IS NOT NULL AND sale_return_item_id IS NOT NULL)
);CREATE UNIQUE INDEX uk_inventory_entry_sale_return_item ON inventory_entry(sale_return_item_id,entry_type) WHERE sale_return_item_id IS NOT NULL;
CREATE INDEX idx_inventory_entry_sale_return ON inventory_entry(sale_return_id,id);
ALTER TABLE partner_balance_entry ADD COLUMN sale_return_id BIGINT REFERENCES sale_return_document(id);
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='PURCHASE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NOT NULL AND sale_return_id IS NULL) OR
 (entry_type='SALE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
CREATE UNIQUE INDEX uk_partner_balance_sale_return ON partner_balance_entry(sale_return_id) WHERE sale_return_id IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS uk_partner_balance_sale_return;
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL AND purchase_return_id IS NULL) OR
 (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL AND purchase_return_id IS NULL) OR
 (entry_type='PURCHASE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
ALTER TABLE partner_balance_entry DROP COLUMN sale_return_id;
DROP INDEX IF EXISTS uk_inventory_entry_sale_return_item;
DROP INDEX IF EXISTS idx_inventory_entry_sale_return;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_source_check;
ALTER TABLE inventory_entry DROP CONSTRAINT inventory_entry_reason_check;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_reason_check CHECK(reason IN ('OPENING','SURPLUS','SHORTAGE','DAMAGE','OTHER','PURCHASE','PURCHASE_CANCEL','SALE','SALE_CANCEL','PURCHASE_RETURN','PURCHASE_RETURN_CANCEL'));
ALTER TABLE inventory_entry ALTER COLUMN reason TYPE VARCHAR(20);
ALTER TABLE inventory_entry DROP COLUMN sale_return_item_id;
ALTER TABLE inventory_entry DROP COLUMN sale_return_id;
ALTER TABLE inventory_entry ADD CONSTRAINT inventory_entry_source_check CHECK (
 (adjustment_id IS NOT NULL AND adjustment_item_id IS NOT NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NOT NULL AND purchase_item_id IS NOT NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NOT NULL AND sale_item_id IS NOT NULL AND purchase_return_id IS NULL AND purchase_return_item_id IS NULL) OR
 (adjustment_id IS NULL AND adjustment_item_id IS NULL AND purchase_id IS NULL AND purchase_item_id IS NULL AND sale_id IS NULL AND sale_item_id IS NULL AND purchase_return_id IS NOT NULL AND purchase_return_item_id IS NOT NULL)
);
