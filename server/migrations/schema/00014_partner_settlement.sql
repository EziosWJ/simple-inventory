-- +goose Up
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_check_2;
ALTER TABLE partner_balance_entry ADD COLUMN payment_method VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE partner_balance_entry ADD COLUMN transaction_no VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_type_check CHECK (
 (entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR
 (entry_type IN ('RECEIPT','PAYMENT') AND amount_cents<0 AND reverses_id IS NULL) OR
 (entry_type='REVERSAL' AND amount_cents<>0 AND reverses_id IS NOT NULL)
);
-- +goose Down
ALTER TABLE partner_balance_entry DROP CONSTRAINT partner_balance_entry_type_check;
ALTER TABLE partner_balance_entry DROP COLUMN transaction_no;
ALTER TABLE partner_balance_entry DROP COLUMN payment_method;
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_check_2 CHECK ((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<0 AND reverses_id IS NOT NULL));
