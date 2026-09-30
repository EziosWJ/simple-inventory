-- +goose Up
INSERT OR IGNORE INTO warehouse(singleton_id,name,remark) VALUES (1,'默认仓库',NULL);
-- +goose Down
DELETE FROM warehouse WHERE singleton_id=1;
