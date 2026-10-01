# 05：当前库存分页查询、零库存视图与停用商品展示

**GitHub issue:** [#12](https://github.com/EziosWJ/simple-inventory/issues/12) · `ready-for-agent`

## Parent

Phase 3 库存基础规格：https://github.com/EziosWJ/simple-inventory/issues/7

## What to build

经营者通过当前库存菜单查看实际数量，默认看所有非零库存且包含停用商品，切换查看全部或零库存并筛选商品。查询读取过账产生的真实余额，无余额记录的实物按零显示，可独立演示库存查询。

## Acceptance criteria

- [x] 交付当前库存菜单、认证分页 API 和列表，展示编码、名称、型号、规格、分类、基本单位、当前数量、启用状态；不展示成本/金额，服务项目始终排除。
- [x] 默认非零库存包含启用与停用实物；可切换全部/零库存，未建立余额记录的实物按零显示，不为显示列表而写入虚假余额。
- [x] 支持编码/名称/型号/规格等商品关键词、分类与启用状态筛选；分页总数及结果使用同一筛选，排序稳定，小数按最多三位准确表达。
- [x] 当前库存展示当前商品资料，和历史快照概念区分；有库存商品改名后该列表显示新描述，余额数值保持。
- [x] 从已保存调整草稿过账后刷新可看到真实增减，草稿保存无影响；双库公开场景覆盖无记录零库存、非零/全部/零切换、同名规格、停用商品、服务排除和分页边界。
- [x] 查询不提供直接覆盖库存、删除流水或成本核算入口；商品流水链接已由 #13 追溯任务接入，链接可用（`/business/inventory-entries?productId=<内部ID>`）。
- [x] 本任务仅读取现有余额，不依赖已过账取消实现；后续取消更新同一余额即可在本页面观察，不额外建立投影或缓存体系。
- [x] 沿用现有登录认证和已登录账号访问范围，记录实际操作人；真实菜单按现有角色菜单规则接入，不使用 mock 权限充当后端授权。
- [x] 业务修改和操作审计同事务，失败整体回滚；如新增 schema/seed，使用双库逻辑版本同步的 Goose 迁移，不改写旧版本。
- [x] 公开 HTTP 行为和真实数据库验证在 PostgreSQL/文件型 SQLite 上通过；新增 contract 确实被现有数据库门禁选中，跳过与未执行项如实报告。
- [x] API 源注解与 Swagger 重新生成，实际页面路径可演示；执行后端 tests+vet、前端 lint/build 及与改动对应的双库检查，报告结果。

## Blocked by

- #10：库存调整整单过账、非负余额与商品引用锁定（https://github.com/EziosWJ/simple-inventory/issues/10）

## 本任务实现与检查记录

- 后端：`Balance`/`BalanceQuery`/`BalancePage` 读模型，`Store.BalancePage`，`Service.BalancePage` 入参校验（页码/每页上限 500、stock 仅接受 nonzero/all/zero、keyword/category 长度上限），`Repository.BalancePage` 使用 `product LEFT JOIN inventory_balance` 以区分“有余额行且为 0”与“从未建立余额行”两种情况。
- 迁移：新增 `seed/00008`（PostgreSQL 与 SQLite 同版本、内容一致），创建内置菜单 `当前库存`（`/business/inventory-balances`）与 `business:inventory-balance` 权限，并授予 ADMIN 角色。
- 前端：`web/src/pages/business/inventory-balances.tsx`，路由 `/business/inventory-balances`，标题映射 `当前库存`。
- Contract：`inventory_balance_contract_test.go` 的 `TestSQLiteInventoryBalanceContract` / `TestPostgresInventoryBalanceContract`，被 `db:integration:sqlite` 与 `db:integration:postgres` 门禁选中。
- 已执行：`go test ./...`、`go vet ./...`、SQLite 与 PostgreSQL 双库门禁、`task frontend:lint`、`task frontend:build`、`task check`、`task db:check` 全部通过。
- 未执行项：无（Docker 可用，PostgreSQL contract 实际执行而非跳过）。
