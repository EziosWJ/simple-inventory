-- +goose Up
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'往来余额','MENU','/business/partner-balances','business/partner-balances','Wallet','business:partner-balance',40,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/partner-balances' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m WHERE r.role_code='ADMIN' AND m.path='/business/partner-balances'
AND NOT EXISTS(SELECT 1 FROM sys_role_menu x WHERE x.role_id=r.id AND x.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path='/business/partner-balances');
DELETE FROM sys_menu WHERE path='/business/partner-balances';
