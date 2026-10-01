-- +goose Up
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type IN ('CUSTOMER_REFUND','SUPPLIER_REFUND') AND ((entry_type='CUSTOMER_REFUND' AND direction='CUSTOMER') OR (entry_type='SUPPLIER_REFUND' AND direction='SUPPLIER')) AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='PURCHASE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NOT NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='SALE' AND amount_cents>0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NOT NULL AND purchase_return_id IS NULL AND sale_return_id IS NULL) OR
 (entry_type='PURCHASE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NOT NULL AND sale_return_id IS NULL) OR
 (entry_type='SALE_RETURN' AND amount_cents<0 AND reverses_id IS NULL AND purchase_id IS NULL AND sale_id IS NULL AND purchase_return_id IS NULL AND sale_return_id IS NOT NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
-- +goose Down
SELECT 1;
