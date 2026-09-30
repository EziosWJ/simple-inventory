-- +goose Up
INSERT INTO sys_menu (parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'商品与服务','MENU','/business/products','business/products','Package','business:product',10,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/products' AND deleted=0);
INSERT INTO sys_menu (parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'往来单位','MENU','/business/partners','business/partners','ContactRound','business:partner',20,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/partners' AND deleted=0);
INSERT INTO sys_menu (parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT 0,'仓库','MENU','/business/warehouse','business/warehouse','Warehouse','business:warehouse',30,1,1,1
WHERE NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/business/warehouse' AND deleted=0);
INSERT INTO sys_role_menu (role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path IN ('/business/products','/business/partners','/business/warehouse')
AND NOT EXISTS (SELECT 1 FROM sys_role_menu x WHERE x.role_id=r.id AND x.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path IN ('/business/products','/business/partners','/business/warehouse'));
DELETE FROM sys_menu WHERE path IN ('/business/products','/business/partners','/business/warehouse');
