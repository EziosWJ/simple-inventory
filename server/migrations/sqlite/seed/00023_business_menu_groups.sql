-- +goose Up
-- Navigation-only parent directories. Leaf paths, ids and permissions stay unchanged.
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'基础资料','DIR','/business/group/basic',NULL,'Package',NULL,10,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/group/basic' AND deleted=0);
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'采购管理','DIR','/business/group/purchases',NULL,'ClipboardList',NULL,20,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/group/purchases' AND deleted=0);
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'销售管理','DIR','/business/group/sales',NULL,'FileText',NULL,30,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/group/sales' AND deleted=0);
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'库存管理','DIR','/business/group/inventory',NULL,'Boxes',NULL,40,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/group/inventory' AND deleted=0);
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'往来管理','DIR','/business/group/partners',NULL,'ContactRound',NULL,50,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/business/group/partners' AND deleted=0);

UPDATE sys_menu SET parent_id=(SELECT id FROM sys_menu WHERE path='/business/group/basic' AND deleted=0)
WHERE path IN ('/business/products','/business/partners','/business/warehouse','/business/print-profile') AND deleted=0;
UPDATE sys_menu SET parent_id=(SELECT id FROM sys_menu WHERE path='/business/group/purchases' AND deleted=0)
WHERE path IN ('/business/purchases','/business/purchase-returns') AND deleted=0;
UPDATE sys_menu SET parent_id=(SELECT id FROM sys_menu WHERE path='/business/group/sales' AND deleted=0)
WHERE path IN ('/business/sales','/business/sale-returns') AND deleted=0;
UPDATE sys_menu SET parent_id=(SELECT id FROM sys_menu WHERE path='/business/group/inventory' AND deleted=0)
WHERE path IN ('/business/inventory-balances','/business/inventory-entries','/business/inventory-adjustments') AND deleted=0;
UPDATE sys_menu SET parent_id=(SELECT id FROM sys_menu WHERE path='/business/group/partners' AND deleted=0)
WHERE path IN ('/business/partner-balances','/business/refunds','/business/partner-ledger') AND deleted=0;

-- Add navigation ancestors only for roles already granted at least one leaf.
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT DISTINCT rm.role_id,p.id
FROM sys_role_menu rm
JOIN sys_menu leaf ON leaf.id=rm.menu_id AND leaf.deleted=0
JOIN sys_menu p ON p.path=CASE
 WHEN leaf.path IN ('/business/products','/business/partners','/business/warehouse','/business/print-profile') THEN '/business/group/basic'
 WHEN leaf.path IN ('/business/purchases','/business/purchase-returns') THEN '/business/group/purchases'
 WHEN leaf.path IN ('/business/sales','/business/sale-returns') THEN '/business/group/sales'
 WHEN leaf.path IN ('/business/inventory-balances','/business/inventory-entries','/business/inventory-adjustments') THEN '/business/group/inventory'
 WHEN leaf.path IN ('/business/partner-balances','/business/refunds','/business/partner-ledger') THEN '/business/group/partners'
 END AND p.deleted=0
WHERE leaf.path IN ('/business/products','/business/partners','/business/warehouse','/business/print-profile',
 '/business/purchases','/business/purchase-returns','/business/sales','/business/sale-returns',
 '/business/inventory-balances','/business/inventory-entries','/business/inventory-adjustments',
 '/business/partner-balances','/business/refunds','/business/partner-ledger')
AND NOT EXISTS(SELECT 1 FROM sys_role_menu existing WHERE existing.role_id=rm.role_id AND existing.menu_id=p.id);

-- +goose Down
UPDATE sys_menu SET parent_id=0
WHERE path IN ('/business/products','/business/partners','/business/warehouse','/business/print-profile',
 '/business/purchases','/business/purchase-returns','/business/sales','/business/sale-returns',
 '/business/inventory-balances','/business/inventory-entries','/business/inventory-adjustments',
 '/business/partner-balances','/business/refunds','/business/partner-ledger') AND deleted=0;
UPDATE sys_menu SET parent_id=0 WHERE parent_id IN (
 SELECT id FROM sys_menu WHERE path IN ('/business/group/basic','/business/group/purchases','/business/group/sales','/business/group/inventory','/business/group/partners') AND menu_type='DIR' AND is_builtin=1
) AND deleted=0;
DELETE FROM sys_role_menu WHERE menu_id IN (
 SELECT id FROM sys_menu WHERE path IN ('/business/group/basic','/business/group/purchases','/business/group/sales','/business/group/inventory','/business/group/partners') AND menu_type='DIR' AND is_builtin=1
);
DELETE FROM sys_menu WHERE path IN (
 '/business/group/basic','/business/group/purchases','/business/group/sales','/business/group/inventory','/business/group/partners'
) AND menu_type='DIR' AND is_builtin=1;
