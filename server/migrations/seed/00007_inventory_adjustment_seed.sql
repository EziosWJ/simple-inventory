-- +goose Up
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,status,visible,is_builtin)
SELECT 0,'库存调整','MENU','/business/inventory-adjustments','business/inventory-adjustments','ClipboardList','business:inventory-adjustment',40,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/inventory-adjustments' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path='/business/inventory-adjustments' AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
INSERT INTO sys_dict_type(dict_name,dict_code,status,sort_order,is_builtin,remark) SELECT '库存调整状态','INVENTORY_ADJUSTMENT_STATUS',1,14,1,'库存调整业务字典' WHERE NOT EXISTS(SELECT 1 FROM sys_dict_type WHERE dict_code='INVENTORY_ADJUSTMENT_STATUS');
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'草稿','DRAFT',1 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_STATUS' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='DRAFT' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'已过账','POSTED',2 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_STATUS' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='POSTED' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'已取消','CANCELLED',3 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_STATUS' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='CANCELLED' AND d.deleted=0);
INSERT INTO sys_dict_type(dict_name,dict_code,status,sort_order,is_builtin,remark) SELECT '库存调整原因','INVENTORY_ADJUSTMENT_REASON',1,15,1,'库存调整业务字典' WHERE NOT EXISTS(SELECT 1 FROM sys_dict_type WHERE dict_code='INVENTORY_ADJUSTMENT_REASON');
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'期初录入','OPENING',1 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_REASON' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='OPENING' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'盘盈','SURPLUS',2 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_REASON' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='SURPLUS' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'盘亏','SHORTAGE',3 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_REASON' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='SHORTAGE' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'报损','DAMAGE',4 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_REASON' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='DAMAGE' AND d.deleted=0);
INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value,sort_order) SELECT t.id,'其他','OTHER',5 FROM sys_dict_type t WHERE t.dict_code='INVENTORY_ADJUSTMENT_REASON' AND t.is_builtin=1 AND NOT EXISTS(SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id=t.id AND d.dict_value='OTHER' AND d.deleted=0);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path='/business/inventory-adjustments' AND is_builtin=1);
DELETE FROM sys_menu WHERE path='/business/inventory-adjustments' AND is_builtin=1;
DELETE FROM sys_dict_data WHERE dict_type_id IN (SELECT id FROM sys_dict_type WHERE dict_code IN ('INVENTORY_ADJUSTMENT_STATUS','INVENTORY_ADJUSTMENT_REASON') AND is_builtin=1);
DELETE FROM sys_dict_type WHERE dict_code IN ('INVENTORY_ADJUSTMENT_STATUS','INVENTORY_ADJUSTMENT_REASON') AND is_builtin=1;
