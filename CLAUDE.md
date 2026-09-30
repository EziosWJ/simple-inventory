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

## 开发规则

1. 只实现当前需求，不提前引入复杂架构。
2. 后端按业务模块组织，保持 Handler → Service → Repository 边界。
3. Service 不依赖 Gin Context、GORM 或具体数据库。
4. Schema 变更必须使用 Goose migration，不使用 `AutoMigrate()`。
5. PostgreSQL/SQLite 差异必须局部化到 database、repository 或 migration。
6. 修改 REST API 时同步 Swagger。
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
