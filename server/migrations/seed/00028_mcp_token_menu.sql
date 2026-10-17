-- +goose Up
INSERT INTO sys_menu(parent_id,menu_name,menu_type,path,component,icon,permission_code,sort_order,visible,status,is_builtin)
SELECT (SELECT id FROM sys_menu WHERE path='/system' AND deleted=0),'MCP令牌','MENU','/system/mcp-token','system/mcp-token/index','KeyRound','system:mcp-token',7,1,1,1
WHERE NOT EXISTS(SELECT 1 FROM sys_menu WHERE path='/system/mcp-token' AND deleted=0);
INSERT INTO sys_role_menu(role_id,menu_id)
SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m
WHERE r.role_code='ADMIN' AND m.path='/system/mcp-token' AND m.deleted=0
AND NOT EXISTS(SELECT 1 FROM sys_role_menu rm WHERE rm.role_id=r.id AND rm.menu_id=m.id);
-- +goose Down
DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE path='/system/mcp-token' AND is_builtin=1);
DELETE FROM sys_menu WHERE path='/system/mcp-token' AND is_builtin=1;
