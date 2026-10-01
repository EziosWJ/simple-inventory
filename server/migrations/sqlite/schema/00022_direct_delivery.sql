-- +goose Up
ALTER TABLE purchase_document ADD COLUMN direct_delivery BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE sale_document ADD COLUMN direct_delivery BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE sale_document ADD COLUMN direct_purchase_id BIGINT REFERENCES purchase_document(id);
CREATE UNIQUE INDEX idx_direct_delivery_active_purchase ON sale_document(direct_purchase_id) WHERE direct_purchase_id IS NOT NULL AND status <> 'CANCELLED';
CREATE INDEX idx_direct_delivery_history ON sale_document(direct_purchase_id,id);
-- +goose Down
DROP INDEX idx_direct_delivery_history;
DROP INDEX idx_direct_delivery_active_purchase;
ALTER TABLE sale_document DROP COLUMN direct_purchase_id;
ALTER TABLE sale_document DROP COLUMN direct_delivery;
ALTER TABLE purchase_document DROP COLUMN direct_delivery;
