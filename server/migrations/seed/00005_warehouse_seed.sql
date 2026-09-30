-- +goose Up
INSERT INTO warehouse(singleton_id,name,remark) VALUES (1,'默认仓库',NULL) ON CONFLICT (singleton_id) DO NOTHING;
-- +goose Down
DELETE FROM warehouse WHERE singleton_id=1;
