# MCP V1 实施与验证记录

日期：2026-10-15。规格基线：`c0eb9e4`（设计 PR #91）。开发分支：`feat/private-mcp-v1`。本记录只涉及隔离数据，不代表生产启用或部署。

## 工作包

| 工作包 | 状态 | 交付 |
| --- | --- | --- |
| T01 基础与授权 | 已实现，定向验证通过 | 默认关闭、SDK HTTP、实时权限交集、摘要令牌、CLI、双库 migration 30、安全边界与资源限制；局域网补充见文末 |
| T02 四只读工具 | 已实现，定向验证通过 | 显式 Schema、Service 适配、分页和输出白名单、对象与余额语义、统一脱敏错误 |
| T03 集成与试用 | 已实现并通过自动验证；桌面实机待人工 | 双库 HTTP/SDK 与升级契约、样例脚本、CLI 指南、JS 客户端 smoke、Cherry 人工清单 |
| Cherry Studio 桌面实机 | 待人工完成 | 当前工具无法操作桌面 Cherry；不将 SDK 验证视为桌面验收 |

## 已执行验证

- `task db:contract -- '^TestSQLiteMCP'`：通过；先完成 SQLite 修复，再执行 PostgreSQL。
- `task db:contract -- '^TestPostgresMCP'`：通过，独立 `postgres:17-alpine` 容器，非 SKIP。
- 最终增补用例后，`task db:contract -- '^Test(SQLite|Postgres)MCP'`：通过（约13.1秒）。
- 新增四个契约：`TestSQLiteMCPHTTPContract`、`TestPostgresMCPHTTPContract`、`TestSQLiteMCPMigrationUpgrade`、`TestPostgresMCPMigrationUpgrade`。HTTP 契约从空库完整迁移；升级契约从29版保留旧账号／商品升级到30，并回退再升级。
- `go test ./cmd/mcp-token ./internal/config ./internal/mcpserver`：通过。覆盖令牌管理 CLI、配置默认与边界、非本机连接、Origin/Host/转发头、预检、限流、并发、真实超时占槽、错误脱敏、旧协议批量扇出阻断和输出限制。
- `go test -tags=embedweb ./internal/app ./internal/webui`：通过；使用现有 `web/dist` 复制到被 Git 忽略的嵌入目录，包含禁用 MCP 不被 SPA/CORS 吞掉的验收。
- 最终 embedweb API 和令牌 CLI 二进制构建通过（产物仅在 `/tmp`）；实际 HTTP 验证 `/`、`/business/products` 与 `/health` 通过，最终二进制上的 JS smoke 通过。
- `scripts/mcp-demo.sh`：通过；创建新临时 SQLite 和独立测试身份、样例，不读生产数据。
- 在样例 API 上 `scripts/mcp-client-smoke.mjs`：官方 JS SDK 1.17.5 通过真实连接、四工具发现、同名候选、12.5支库存、123.45/-67.89 双向余额和错误凭证401。所有 SDK 请求都实际携带 Bearer，后续请求协议头为2025-06-18。
- 官方 Go SDK 客户端通过真实 HTTP 调用；原始 initialize 另验证2025-03-26、2025-06-18、2025-11-25协商。
- 权限契约覆盖错误／过期／吊销凭证，账号停用／软删除、角色停用、菜单授权撤回、scope 交集；同一 SDK 客户端下一请求立即更新列表与调用权限。
- 工具契约覆盖未知字段、null、枚举、非法ID、页码0/1001、页大小51、同名候选、服务、零库存、对象不存在、无记录／已有记录零余额、客户与供应商正负余额、历史方向及不存在方向、输出禁用字段。

完整日志存放于本次开发机 `/tmp/mcp-*.log`；日志与凭据均不纳入版本控制。

## 最终数据库门禁

一次 `task db:check` **退出码0通过**（日志 `/tmp/simple-inventory-mcp-db-check.log`）：后端 tests/vet、SQLite全量35.909秒、PostgreSQL全量403.170秒。包含现有 REST、销售送货单、对账单／打印资料、Excel导出、附件及 PostgreSQL备份恢复等既有契约。

最终复核补充空 permission_code 菜单兼容、工具 arguments 省略的空对象处理及说明、旧协议 batch 单RPC限制后，重新按顺序通过 `TestSQLiteMCPHTTPContract`（0.485秒）和 `TestPostgresMCPHTTPContract`（5.724秒），并通过 `task backend:check`、embedweb app/webui 检查。migration、既有业务、数据库基础设施与依赖版本未再变化，复用上述全量门禁，不重复运行。

`go test -race ./internal/mcpserver` 通过（1.078秒）；并发与超时槽的测试没有数据竞争。SQLite C依赖编译出现既有 const qualifier warning，不影响退出码。

`go test -tags=integration ./integration -list '^Test(SQLite|Postgres)MCP'` 确认四项均匹配；此列表检查仅用于选择器核对，不替代上面的实际执行。

Taskfile 的 SQLite 选择器 `^TestSQLite` 与 PostgreSQL 选择器 `^Test(Postgres|...)` 各选中新契约一次；CI `.github/workflows/database-compatibility.yml` 使用相同分库前缀。新增测试没有借用模糊的业务模块前缀，也没有重复挂入同一个数据库门禁。

环境替代方案：WSL 原 Docker 入口不可用，启动已安装的 Docker Desktop；本次命令通过 `/tmp/mcp-test-bin/docker` 转发到 Windows Docker CLI。备份回归所需 `pg_dump`/`psql` 通过临时适配器在该用例的独立 PostgreSQL17容器内真实执行，按临时容器发布端口匹配，仅接受 integration 测试 DSN。未修改生产网络、数据库或仓库测试逻辑以绕过验收。CI 继续使用标准 Docker 和 PostgreSQL17客户端。

## 未执行与限制

- Cherry Studio 桌面实机：当前环境无可交互的桌面客户端工具；实际版本待人工记录，按[逐步清单](mcp-server-v1-usage.md#逐步人工试用清单)完成正常连接、错误凭据、三个业务场景及模型选择行为。
- 未修改 REST 合同或注解，故未运行 `task api:docs`；MCP JSON-RPC 不生成虚构 Swagger REST 定义。
- 未修改前端源码，故未运行 frontend lint/build；嵌入兼容检查复用现有构建产物，不宣称重新验证前端构建。
- 不运行生产部署或真实业务接入；默认关闭，开发 PR 不自动合并。
- 资源限制是单 API 进程内限制；SQLite 继续仅支持单实例。外部模型是否遵守候选选择与数据指令隔离提示仍需人工试用，服务端查询本身严格要求内部 ID。

## 手机／局域网测试补充

用户于2026-10-15要求本地测试允许手机访问，撤销仅本机限制。MCP现在允许 `0.0.0.0` 监听和非回环直连，Host白名单允许明确的局域网IP／主机名；令牌、权限、Origin、转发头和资源限制继续生效。样例脚本默认监听 `0.0.0.0:18300`，用 `MCP_DEMO_HOST=<电脑局域网IP>` 添加连接地址白名单。

本次通过：

- `task backend:check`：tests/vet通过，配置与入口回归覆盖局域网来源／Host成功、错误凭证401及非法Host／伪造代理头拒绝。
- 定向 `TestSQLiteMCPHTTPContract` 通过（1.977秒），随后 `TestPostgresMCPHTTPContract` 通过（5.906秒）。
- `go test -race ./internal/mcpserver` 通过（1.080秒）。
- 样例脚本成功生成 `0.0.0.0:18300` 环境；实际API监听全接口，通过开发机真实非回环私网地址调用 `scripts/mcp-client-smoke.mjs`，Bearer、2025-06-18协商、四工具、候选、库存、双向余额及错误凭证全部通过。
- `bash -n scripts/mcp-demo.sh`、`node --check scripts/mcp-client-smoke.mjs`、`git diff --check`通过。

本次仅改MCP配置及连接边界，没有schema、数据库基础设施、业务查询或REST合同变化，因此复用此前完整双库门禁，不重复 `task db:check`；不运行Swagger生成或前端检查。真实手机及Cherry Studio由用户独立测试，不能将开发机私网地址smoke视作手机已验收。测试后停止本次临时API并吊销本次凭证，不修改生产网络或账号。
