# MCP V1 验证记录

规格基线：`c0eb9e4`（设计 PR #91），开发分支：`feat/private-mcp-v1`。

## 已通过

- T01：SDK HTTP、配置默认关闭、独立摘要令牌、CLI、实时账号／角色菜单权限与scope交集、双库migration30。
- T02：四个只读工具的Schema、Service适配、分页、字段白名单、库存及往来余额语义、错误脱敏。
- T03：真实HTTP／官方SDK协议、双库契约、空库迁移、29→30保留旧数据升级及回退再升级。
- 一次 `task db:check`：后端tests/vet、SQLite全量35.909秒、PostgreSQL全量403.170秒；包括现有API、打印、Excel导出、附件与备份恢复。
- 最终代码调整按顺序通过定向SQLite、PostgreSQL MCP HTTP契约；后端检查、race及embedweb app/webui兼容检查通过。
- 官方JS SDK1.17.5的真实HTTP调用通过：认证头、2025-06-18协商、四工具、同名候选、库存、正负双向余额及错误凭证。
- `0.0.0.0`监听、开发机真实私网地址访问通过；手机实机由用户独立测试。

四个新增数据库契约分别为 `TestSQLiteMCPHTTPContract`、`TestPostgresMCPHTTPContract`、`TestSQLiteMCPMigrationUpgrade`、`TestPostgresMCPMigrationUpgrade`，Taskfile与CI分库选择器各匹配一次。

## 开发入口简化

直接使用 `task dev:sqlite` 启动前后端及MCP，复用现有开发库。`task mcp:token:sqlite` 签发本地用户的连接凭证。开发任务使用 `0.0.0.0:8300` 与Host通配符，无需额外环境文件或IP白名单。

已移除手工样例生成脚本及其专用客户端脚本。自动化测试保留正常测试夹具，不影响日常启动流程。本次 `task backend:check` 通过；实际运行 `task dev:sqlite` 与 `task mcp:token:sqlite`，本机和真实私网地址上的认证工具发现均返回四工具，无凭证返回401，前端5173可访问。`git diff --check`通过。

## 未执行

- Cherry Studio与手机实机：由用户独立完成，未将SDK调用标记为客户端实机验收。
- 未改REST合同，不运行Swagger生成；未改前端，不运行前端lint/build。
- 本次没有数据库schema或业务行为变化，复用上述全量门禁，不重复运行全量双库检查。
