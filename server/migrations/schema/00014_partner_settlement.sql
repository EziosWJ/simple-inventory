-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    opening_reversal_check TEXT;
    matching_checks INTEGER;
    opening_reversal_type_check TEXT;
    matching_type_checks INTEGER;
BEGIN
    SELECT COUNT(*), MIN(conname)
      INTO matching_checks, opening_reversal_check
      FROM pg_constraint
     WHERE conrelid = 'partner_balance_entry'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) ILIKE '%OPENING%'
       AND pg_get_constraintdef(oid) ILIKE '%REVERSAL%'
       AND pg_get_constraintdef(oid) ILIKE '%amount_cents > 0%'
       AND pg_get_constraintdef(oid) ILIKE '%reverses_id IS NULL%'
       AND pg_get_constraintdef(oid) NOT ILIKE '%RECEIPT%';
    IF matching_checks <> 1 THEN
        RAISE EXCEPTION 'expected one opening/reversal check on partner_balance_entry, found %', matching_checks;
    END IF;
    EXECUTE format('ALTER TABLE partner_balance_entry DROP CONSTRAINT %I', opening_reversal_check);
    SELECT COUNT(*), MIN(conname)
      INTO matching_type_checks, opening_reversal_type_check
      FROM pg_constraint
     WHERE conrelid = 'partner_balance_entry'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) ILIKE '%OPENING%'
       AND pg_get_constraintdef(oid) ILIKE '%REVERSAL%'
       AND pg_get_constraintdef(oid) NOT ILIKE '%amount_cents%';
    IF matching_type_checks <> 1 THEN
        RAISE EXCEPTION 'expected one opening/reversal type check on partner_balance_entry, found %', matching_type_checks;
    END IF;
    EXECUTE format('ALTER TABLE partner_balance_entry DROP CONSTRAINT %I', opening_reversal_type_check);
END $$;
-- +goose StatementEnd
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
ALTER TABLE partner_balance_entry ADD CONSTRAINT partner_balance_entry_opening_reversal_check CHECK ((entry_type='OPENING' AND amount_cents>0 AND reverses_id IS NULL) OR (entry_type='REVERSAL' AND amount_cents<0 AND reverses_id IS NOT NULL));
