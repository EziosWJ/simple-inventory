-- +goose Up
UPDATE sys_menu SET menu_name='退款',sort_order=53 WHERE path='/business/refunds' AND deleted=0;
UPDATE sys_menu SET menu_name='往来明细',sort_order=51 WHERE path='/business/partner-ledger' AND deleted=0;
UPDATE sys_menu SET sort_order=50 WHERE path='/business/partner-balances' AND deleted=0;

INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT p.id,'收付款','MENU','/business/settlements','business/settlements','Wallet','business:settlement',52,1,1,1
FROM sys_menu p WHERE p.path='/business/group/partners' AND p.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/settlements' AND deleted=0);
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT p.id,'期初录入','MENU','/business/opening-balances','business/opening-balances','Wallet','business:opening-balance',54,1,1,1
FROM sys_menu p WHERE p.path='/business/group/partners' AND p.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/opening-balances' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path IN ('/business/settlements','/business/opening-balances') AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);

-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path IN ('/business/settlements','/business/opening-balances'));
DELETE FROM sys_menu WHERE path IN ('/business/settlements','/business/opening-balances');
UPDATE sys_menu SET menu_name='退款结算',sort_order=41 WHERE path='/business/refunds' AND deleted=0;
UPDATE sys_menu SET menu_name='往来明细与对账',sort_order=42 WHERE path='/business/partner-ledger' AND deleted=0;
UPDATE sys_menu SET sort_order=40 WHERE path='/business/partner-balances' AND deleted=0;
