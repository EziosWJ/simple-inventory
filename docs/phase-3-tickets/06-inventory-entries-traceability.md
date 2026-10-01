# 06：库存流水分页查询、原始与冲销筛选及来源追溯

**GitHub issue:** [#13](https://github.com/EziosWJ/simple-inventory/issues/13) · `ready-for-agent`

## Parent

Phase 3 库存基础规格：https://github.com/EziosWJ/simple-inventory/issues/7

## What to build

经营者从当前库存进入某商品的流水，筛选原始变动或取消冲销，查看数量前后变化、原因和操作人，再打开来源调整单完成追溯。该任务贯通原始过账、取消冲销、当前库存与历史单据页面。

## Acceptance criteria

- [x] 交付库存流水菜单、认证分页 API 和列表，按商品、发生时间、原始变动/取消冲销筛选，确定性排序，分页和总数一致。
- [x] 展示生效时间、过账时商品编码/名称/型号/规格/单位快照、增减数量、前后余额、原因/说明、实际操作人、来源单号；历史描述不随当前档案编辑变化。
- [x] 反向流水可识别取消冲销及关联原变动/原单据，原始流水仍保留；详情可查看实际取消原因，不把创建人当作执行人。
- [x] 当前库存的商品操作链接进入预先按商品内部 ID 筛选的流水；即使商品改名、停用或余额归零，全部历史仍可查。
- [x] 流水可打开来源调整单，查看全部明细及创建/过账/取消记录；调整列表按商品筛选不重复计数，并包含已取消单据。
- [x] 原始和冲销时间范围、时区表达与前端展示一致；同一时间多条记录的排序保持稳定，不支持倒签或历史重算。
- [x] 双库公开行为覆盖原始/冲销筛选、时间边界、前后余额、来源链接、快照、停用/改名/归零历史与操作人；实际页面演示库存→流水→来源单据。
- [x] 完成整阶段用户路径验收和回归：新建/编辑草稿→混合增减过账→查库存/流水→取消→查冲销；验证余额等于原始与反向流水累计，记录检查结果和未执行项。
- [x] 本任务不新增流水写入通道或读模型同步机制；复用前序任务的真实库存记录，不交付导出、打印、报表或 SN 功能。
- [x] 沿用现有登录认证和已登录账号访问范围，记录实际操作人；真实菜单按现有角色菜单规则接入，不使用 mock 权限充当后端授权。
- [x] 业务修改和操作审计同事务，失败整体回滚；如新增 schema/seed，使用双库逻辑版本同步的 Goose 迁移，不改写旧版本。
- [x] 公开 HTTP 行为和真实数据库验证在 PostgreSQL/文件型 SQLite 上通过；新增 contract 确实被现有数据库门禁选中，跳过与未执行项如实报告。
- [x] API 源注解与 Swagger 重新生成，实际页面路径可演示；执行后端 tests+vet、前端 lint/build 及与改动对应的双库检查，报告结果。

## Blocked by

- #11：已过账库存调整取消、反向流水与纠错路径（https://github.com/EziosWJ/simple-inventory/issues/11）
- #12：当前库存分页查询、零库存视图与停用商品展示（https://github.com/EziosWJ/simple-inventory/issues/12）

## 本任务实现与检查记录

- 后端：`Entry.OperatorName`/`Entry.DocumentNo` 只读关联字段；`EntryQuery`/`EntryPage`；`Store.EntryPage` 与 `Service.EntryPage` 入参校验（页码/每页上限 500、entryType 仅 ORIGINAL/REVERSAL、productId>0、时间上下界顺序）。列表使用 `inventory_entry e LEFT JOIN sys_user LEFT JOIN inventory_adjustment`，排序 `occurred_at DESC, id DESC`。
- 时间语义：`occurredFrom` 含、`occurredTo` 不含，均要求 RFC3339 并归一到 UTC；前端日期控件按本地日边界换算为 UTC 瞬间。
- 前端：`web/src/pages/business/inventory-entries.tsx`（路由 `/business/inventory-entries`，支持 `?productId=` 预筛选），共享只读来源单据弹窗 `web/src/pages/business/inventory-adjustment-detail.tsx`；当前库存页新增“查看流水”操作。
- 迁移：`seed/00009` 新增内置菜单 `库存流水`（`/business/inventory-entries`）与 `business:inventory-entry` 权限，双库同版本；SQLite 集成测试菜单数断言更新为 23。
- Contract：`inventory_entry_contract_test.go` 与整阶段验收 `inventory_phase3_acceptance_test.go`，四组测试名均含 `SQLite`/`Postgres`，被现有数据库门禁选中。
- 已执行：`go test ./...`、`go vet ./...`、`db:integration:sqlite`、`db:integration:postgres`、`task check`、`task db:check`、`task frontend:lint`、`task frontend:build`，全部通过；Swagger 已重新生成并包含 `/api/v1/inventory/entries`。
- 停用/改名/归零历史、时间边界、来源链接与操作人均有双库断言覆盖。
- 未执行项：无（Docker 可用，PostgreSQL 实际执行）。
