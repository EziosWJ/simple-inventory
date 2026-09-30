# Simple Inventory 项目上下文

## 产品身份

- 中文名：简单进销存
- 英文名：Simple Inventory
- 仓库：`EziosWJ/simple-inventory`
- 仓库形态：React + Go monorepo
- 后端目录：`server/`
- 前端目录：`web/`
- Go module：`github.com/EziosWJ/simple-inventory/server`
- 前端 package：`simple-inventory-web`
- API service：`simple-inventory-api`
- 默认 PostgreSQL 数据库：`simple_inventory`
- 默认 SQLite 文件：`.data/simple-inventory.db`

## 产品定位

这是一个轻量级进销存系统，目标用户是需要管理商品、采购、销售和库存的小型业务团队。系统优先解决核心进销存闭环，不以 ERP、财务总账、复杂供应链或多组织平台为第一阶段目标。

目标业务模块：

1. 商品与基础资料
2. 客户 / 供应商
3. 仓库
4. 采购入库
5. 销售出库
6. 当前库存
7. 库存流水
8. 库存调整
9. 基础报表
10. 进出库单据打印

目前尚未实现上述进销存领域模块。当前代码提供的是产品基础能力，后续业务开发不得把已有管理后台示例误认为真实进销存领域模型。

## 当前已具备的基础能力

后端已经具备 Gin HTTP 服务、Koanf 配置、PostgreSQL/SQLite、Goose migration、JWT 会话认证、RBAC、用户、角色、菜单、部门、字典、系统配置、本地文件、登录日志、操作审计、站内通知、Prometheus 与 Swagger。

前端已经具备登录、权限菜单、管理布局、通用表格/表单组件以及对应的系统管理页面。

## 目录边界

```text
/
├── server/       # Go REST API
│   ├── cmd/
│   ├── configs/
│   ├── migrations/
│   ├── docs/
│   └── internal/
├── web/          # React SPA
│   └── src/
├── docs/
│   ├── adr/
│   └── agents/
├── CONTEXT.md
├── CLAUDE.md
└── Taskfile.yml
```

## 后端架构约定

后端采用模块化单体（Modular Monolith），不提前拆微服务。

业务代码优先按领域模块组织：

```text
server/internal/
├── product/
├── partner/
├── warehouse/
├── inventory/
├── purchase/
└── sale/
```

模块按实际需要包含 `handler.go`、`service.go`、`repository.go`、`dto.go`、`model.go`，不创建全局 controller/service/repository 技术分层目录。

依赖方向：

```text
Handler → Service → Repository → GORM → database/sql → Database
```

- Handler 负责 HTTP、DTO、参数校验和响应。
- Service 负责业务规则、状态流转、事务编排，不依赖 Gin Context 或具体数据库。
- Repository 负责查询、持久化和数据库事务操作。
- 依赖在 `internal/app` 中显式组装，不使用 Service Locator 或业务 Singleton。

## 数据库原则

正式支持 PostgreSQL 与 SQLite，PostgreSQL 为默认数据库。SQLite 只承诺单 API 实例、本地持久文件和小规模低写并发。

业务层优先使用两者公共能力子集：CRUD、事务、普通 JOIN、GROUP BY、ORDER BY、LIMIT/OFFSET、索引、唯一约束、外键和基础聚合。

数据库专属 SQL 或特性必须隔离在 database/repository/migration 层。Schema 变更必须提交 Goose migration；API 进程不执行 `AutoMigrate()`。

PostgreSQL 与 SQLite 使用独立 migration 树并保持逻辑版本同步。涉及库存、采购、销售等核心数据的修改必须验证事务一致性。

## 进销存核心建模原则

后续实现时遵守以下方向：

- 库存由“当前余额 + 不可变库存流水”共同表达，不能只保存一个可覆盖的 stock 数值。
- 采购/销售单据先采用简单状态流：`DRAFT → POSTED / CANCELLED`。
- 只有过账才改变库存。
- 已过账单据取消通过反向流水处理，不直接删除历史库存记录。
- 销售出库需要在同一数据库事务内完成库存校验、库存扣减、流水写入和单据状态更新。
- 数量和金额使用适合业务精度的定点/decimal 方案，不使用 float 表示金额。
- 单据打印属于产品正式能力，打印数据应来源于已保存的业务单据，而不是仅从页面 DOM 拼装不可追溯内容。

## API 与文档

现有兼容接口继续使用既有 `/api/**` 路径。新增进销存业务接口优先使用 `/api/v1/**`。

修改 REST API 时同步维护 Swagger。Swagger UI 只在开发环境启用。

## 验证要求

- 后端修改：`task backend:check`
- 前端修改：`task frontend:lint` + `task frontend:build`
- 数据库结构修改：同时验证 PostgreSQL 与 SQLite migration/integration contract
- 完整检查：`task check`
- 数据库兼容门禁：`task db:check`

## 阶段规划

- Phase 1：基础工程产品化
- Phase 2：商品、客户/供应商、仓库
- Phase 3：库存余额、库存流水、库存调整
- Phase 4：采购入库、销售出库
- Phase 5：报表、打印和体验完善

Phase 1 不新增进销存业务表或业务接口，只完成项目身份、目录、配置、文档与模板界面的产品化。
