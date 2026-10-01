-- +goose Up
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,status,visible,is_builtin)
SELECT 0,'采购入库','MENU','/business/purchases','business/purchases','ShoppingCart','business:purchase',45,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/purchases' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path='/business/purchases' AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path='/business/purchases' AND is_builtin=1);
DELETE FROM sys_menu WHERE path='/business/purchases' AND is_builtin=1;
