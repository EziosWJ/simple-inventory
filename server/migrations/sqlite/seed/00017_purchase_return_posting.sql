-- +goose Up
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path='/business/purchase-returns' AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE role_id IN (SELECT id FROM sys_role WHERE role_code='ADMIN')
AND menu_id IN (SELECT id FROM sys_menu WHERE path='/business/purchase-returns' AND deleted=0);
