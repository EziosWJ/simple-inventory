-- +goose Up
ALTER TABLE inventory_adjustment ADD COLUMN cancelled_by BIGINT REFERENCES sys_user(id);
ALTER TABLE inventory_adjustment ADD COLUMN cancelled_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE inventory_adjustment ADD COLUMN cancel_reason VARCHAR(500);
ALTER TABLE inventory_adjustment ADD CONSTRAINT ck_inventory_adjustment_cancellation CHECK ((status='CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND cancel_reason IS NOT NULL AND length(trim(cancel_reason)) BETWEEN 1 AND 500) OR (status<>'CANCELLED' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL));
-- +goose Down
ALTER TABLE inventory_adjustment DROP CONSTRAINT ck_inventory_adjustment_cancellation;
ALTER TABLE inventory_adjustment DROP COLUMN cancel_reason;
ALTER TABLE inventory_adjustment DROP COLUMN cancelled_at;
ALTER TABLE inventory_adjustment DROP COLUMN cancelled_by;
