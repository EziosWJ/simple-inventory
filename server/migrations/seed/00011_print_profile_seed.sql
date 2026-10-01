-- +goose Up
INSERT INTO sys_config(config_name,config_key,config_value,config_type,value_type,status,is_builtin,remark)
SELECT '经营者名称','business.print-profile.name','','SYSTEM','TEXT',1,1,'经营者打印资料'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key='business.print-profile.name');
INSERT INTO sys_config(config_name,config_key,config_value,config_type,value_type,status,is_builtin,remark)
SELECT '经营者电话','business.print-profile.phone','','SYSTEM','TEXT',1,1,'经营者打印资料'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key='business.print-profile.phone');
INSERT INTO sys_config(config_name,config_key,config_value,config_type,value_type,status,is_builtin,remark)
SELECT '经营者地址','business.print-profile.address','','SYSTEM','TEXT',1,1,'经营者打印资料'
WHERE NOT EXISTS (SELECT 1 FROM sys_config WHERE config_key='business.print-profile.address');
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,status,visible,is_builtin)
SELECT 0,'经营者打印资料','MENU','/business/print-profile','business/print-profile','Store','business:print-profile',45,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/print-profile' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path='/business/print-profile' AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path='/business/print-profile' AND is_builtin=1);
DELETE FROM sys_menu WHERE path='/business/print-profile' AND is_builtin=1;
DELETE FROM sys_config WHERE config_key IN ('business.print-profile.name','business.print-profile.phone','business.print-profile.address');
