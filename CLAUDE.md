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
- `task db:contract -- '<测试名正则>'`：定向运行集成契约，强制重跑
- `task db:check`：PostgreSQL/SQLite 数据库兼容验证

## 工作节奏

- 开工先核对 `git status --short`、当前需求的规格与依赖，以及 `df -h /tmp`；保留已有未提交改动。并行实现使用独立工作树，先约定文件归属，由一人顺序集成。
- 开工按下表确定验证范围；迭代运行最小相关检查，收尾补齐该范围的检查。涉及数据库时先定向 SQLite，修复完成后再定向 PostgreSQL。
- 同一工作批次复用已通过的检查；只有相关实现、测试、依赖或配置变化，才重跑对应检查。文档修改不使代码检查失效；浏览器验收复用仍匹配当前代码的隔离 API。
- 长检查把完整输出写入 `/tmp` 日志，结束后查看退出码与摘要，失败时只读取相关日志。等待时使用较长间隔并简短报告进度，减少轮询与输出占用。
- 新增双库 contract 分别使用 `TestSQLite...`、`TestPostgres...` 前缀，核对 Taskfile 与 CI 各选中一次。新增 Goose migration 时验证双库空库和旧数据升级，并同步相关迁移版本、菜单数量断言。

## 验证分层

| 改动范围 | 收尾检查 |
| --- | --- |
| 文档、注释 | `git diff --check` |
| 前端页面、组件、样式 | `task frontend:lint`、`task frontend:build`，及相关 `web/tests/*.mjs` |
| 后端内部逻辑，无数据库行为变化 | `task backend:check`；迭代时只跑相关模块测试 |
| REST 路径、参数、响应或注解 | 后端检查 + `task api:docs`，核对生成差异 |
| 局部查询、仓储、库存/采购/销售过账与冲销、往来金额 | 后端检查 + **受影响契约定向双库验证**，包括关联业务及失败回滚/并发场景 |
| migration/schema/约束；公共 database、事务或锁机制；影响范围无法明确界定 | **一次** `task db:check`；迁移另验证双库空库与旧数据升级 |

- 定向双库通过即可完成局部数据库任务；全量数据库门禁用于上表最后一行、发布验收或用户明确要求。CI 保留全量双库门禁。
- 受影响契约按测试实际内容选择，关联场景可能位于其他模块或 `Phase*`、`ZeroAmount`、`EffectiveTime` 等名称下；新增回归测试必须被选中。命令出现无匹配用例或所需用例 SKIP 时，不算验证通过。
- `task db:check` 已包含后端检查；需求明确要求 `task check` 时执行。完成后报告通过项、未执行项及原因，不将定向通过描述为全量通过。
- 命令示例、成本说明与测试基础设施优化建议见 [验证指南](docs/agents/verification.md)；需要选择契约或优化验证流程时阅读。

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
