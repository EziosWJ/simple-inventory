# CLAUDE.md

请使用中文交流。

## 项目

本仓库是“简单进销存（Simple Inventory）”。

```text
simple-inventory/
├── server/   # Go REST API
├── web/      # React 管理端
├── docs/     # ADR 与开发文档
└── Taskfile.yml
```

开始任务前先阅读根目录 `CONTEXT.md`，以其中的产品范围、领域术语和架构约束为准。

## 开发入口

优先使用根目录 `Taskfile.yml`：

- `task db:migrate`：PostgreSQL migration
- `task db:migrate:sqlite`：SQLite migration
- `task api`：启动 Go API
- `task api:sqlite`：使用 SQLite 启动 API
- `task web`：启动 React
- `task dev`：迁移并启动前后端
- `task dev:sqlite`：SQLite 模式迁移并启动前后端
- `task backend:check`：Go tests + vet
- `task frontend:lint`：前端 lint
- `task frontend:build`：前端 build
- `task check`：后端检查 + 前端 lint/build
- `task db:check`：PostgreSQL/SQLite 数据库兼容验证
- `task db:contract -- '<-run 正则>'`：只跑名字匹配的集成契约，避免为单个改动启动几十个容器

## 工作节奏

- 开工先核对 `git status --short`、当前需求的规格与依赖，以及 `df -h /tmp`；保留已有未提交改动。并行实现使用独立工作树，先约定文件归属，由一人顺序集成。
- 调试先运行最小相关检查；涉及数据库时先定向验证 SQLite，再定向验证 PostgreSQL。同一任务内迭代不跑全门禁，收尾时按“验证分层”跑一次该范围的完整门禁；需求明确要求 `task check` 时照常执行。
- 新增双库 contract 分别使用 `TestSQLite...`、`TestPostgres...` 前缀，核对 Taskfile 与 CI 各选中一次。新增 Goose migration 时验证双库空库和旧数据升级，并同步相关迁移版本、菜单数量断言。

## 验证分层

按改动范围选择检查，不要每次改动都跑 `task db:check`：

| 改动范围 | 迭代时 | 收尾门禁 |
| --- | --- | --- |
| 文档、注释、`docs/**`、`*.md` | 不需要 | 无；必要时 `git diff --check` |
| 前端页面、组件、样式 | `task frontend:lint` | `task frontend:lint`、`task frontend:build`，以及该页面对应的 `web/tests/*.mjs` |
| 后端模块内部实现（无 schema、无 REST 注解） | `cd server && go test ./internal/<模块>/... -count=1` | `task backend:check` |
| REST 路径、参数、响应或注解 | 同上 | 加 `task api:docs`，并确认生成差异只有预期内容 |
| 数据库查询或仓储（无迁移） | 定向契约（见下） | `task db:check` |
| Goose migration、schema、唯一约束 | 定向双库单用例 | `task db:check`，并验证双库空库迁移与旧数据升级 |
| 库存、采购、销售过账与冲销、往来金额 | 定向契约 | `task db:check` |

定向运行（只跑本次相关用例）：

```sh
task db:contract -- '^TestSQLiteSale|^TestPostgresSale'                        # 名字前缀匹配
cd server && go test -tags=integration ./integration -run '^TestSQLite' -count=1  # 全部 SQLite 契约
```

本机实测成本（判断该不该跑）：

- `task backend:check` 约 3 秒；`task frontend:lint` 约 4 秒；`task frontend:build` 约 11 秒；`task check` 约 11 秒（Go 缓存命中时）。
- `task api:docs` 约 20 秒。
- SQLite 全量契约 48 个用例约 29 秒（每个约 0.6 秒）。
- PostgreSQL 全量契约约 5 分钟：每个用例单独启动一个 `postgres:17-alpine` 容器并执行迁移，成本几乎都在容器，而不在断言；单个用例定向运行只要约 7 秒。`task db:check` 的时间因此基本等于 PostgreSQL 套件的时间。
- 真实浏览器脚本（隔离 API 与 Vite 就绪时）约 10 秒；准备隔离 API 另需约 20–30 秒，同一工作时段可复用。
- PostgreSQL 契约需要宿主机 `pg_dump` 不低于服务端版本（见 `server/README.md`），缺失时只有依赖它的那条备份用例失败。

## 开发规则

1. 只实现当前需求，不提前引入复杂架构。
2. 后端按业务模块组织，保持 Handler → Service → Repository 边界。
3. Service 不依赖 Gin Context、GORM 或具体数据库。
4. Schema 变更必须使用 Goose migration，不使用 `AutoMigrate()`。
5. PostgreSQL/SQLite 差异必须局部化到 database、repository 或 migration。
6. 修改 REST API 时同步源注解，运行 `task api:docs` 并检查生成差异；以 Taskfile 中固定的命令为准。
7. 不修改 generated 文件，除非其源定义同时修改并重新生成；如果环境无法生成，必须明确说明。
8. 不把示例页面、测试夹具或基础管理模块误当作进销存领域模型。
9. 业务数据修改必须考虑审计与事务一致性，尤其是库存、采购和销售过账。
10. 完成修改后执行与变更范围对应的检查，并报告未执行项。

## 产品命名

- 产品：简单进销存 / Simple Inventory
- Repository：`EziosWJ/simple-inventory`
- Backend：`server`
- Frontend：`web`
- Go module：`github.com/EziosWJ/simple-inventory/server`
- npm package：`simple-inventory-web`
- API service：`simple-inventory-api`
