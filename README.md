# 简单进销存

Simple Inventory 是一个面向小型业务场景的轻量进销存 Web 系统，采用 React + Go 的 monorepo 结构。

当前仓库已经完成基础工程产品化：项目名称、目录、Go module、前端 package、服务标识、数据库默认名、Swagger、登录页与开发文档均已切换为 Simple Inventory。业务模块将在后续阶段逐步实现。

## 产品范围

目标业务范围包括：

- 商品与基础资料
- 客户 / 供应商
- 仓库
- 采购入库
- 销售出库
- 当前库存与库存流水
- 库存调整
- 基础报表
- 进出库单据打印

## 项目结构

```text
simple-inventory/
├── server/       # Go REST API
├── web/          # React 管理端
├── docs/         # ADR 与开发文档
├── CONTEXT.md    # 产品、领域与架构上下文
├── CLAUDE.md     # Agent 开发约定
└── Taskfile.yml  # 统一开发入口
```

## 技术栈

前端：React 19、TypeScript、Vite、Tailwind CSS、Zustand、react-hook-form、zod。

后端：Go 1.26、Gin、GORM、Goose、Koanf、JWT、Prometheus、Swagger。

数据库：PostgreSQL 为默认数据库；SQLite 支持单实例、小规模部署。

## 项目标识

- 仓库：`EziosWJ/simple-inventory`
- 产品名：`简单进销存`
- 英文名：`Simple Inventory`
- Go module：`github.com/EziosWJ/simple-inventory/server`
- npm package：`simple-inventory-web`
- API service：`simple-inventory-api`
- PostgreSQL 默认数据库：`simple_inventory`
- SQLite 默认文件：`server/.data/simple-inventory.db`

## 本地开发

推荐安装 [Task](https://taskfile.dev/) 并从仓库根目录运行。

首次使用 PostgreSQL：

```bash
cd server
cp .env.example .env
# 修改 POSTGRES_PASSWORD 与 APP_JWT__SECRET
docker compose -f docker-compose.dev.yml up --build
```

或复制本机开发配置：

```bash
cd server
cp configs/config.dev.example.yaml configs/config.dev.yaml
```

常用命令：

```text
task db:migrate
task db:migrate:sqlite
task api
task api:sqlite
task web
task dev
task dev:sqlite
task backend:check
task frontend:lint
task frontend:build
task check
task db:check
```

前端默认开发地址为 `http://localhost:5173`。本机直接启动 Go API 时默认监听 `:8099`；Docker Compose API 映射到 `127.0.0.1:8080`。

开发环境首次 migration 会创建内置管理员 `admin / admin123`。该账号仅用于本地开发，部署环境必须调整安全策略。

## 架构原则

后端采用模块化单体，不提前拆分微服务。业务模块按领域组织，并遵循：

```text
Handler → Service → Repository → GORM/database/sql → Database
```

Schema 变更必须使用 Goose migration；API 进程不执行 `AutoMigrate()`。PostgreSQL 与 SQLite 的方言差异应限制在 migration、database 或 repository 层，不扩散到 Service 和 Handler。

详细约定见 [CONTEXT.md](CONTEXT.md) 和 [docs/adr/](docs/adr/)。后端运行说明见 [server/README.md](server/README.md)。
