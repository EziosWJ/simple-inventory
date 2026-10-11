# 销售出库查询与净销售合计交付记录（#66）

状态：本地实现并完成技术验收，已在 `feat/sale-item-query` 分支提交；未推送、未关闭 GitHub issue、未进行经营者试用。

规格见 [销售查询规格](sales-query-spec.md) 与 [GitHub #66](https://github.com/EziosWJ/simple-inventory/issues/66)。本轮只改善销售出库页面，采购、库存、往来等页面与公用搜索栏默认行为不变。

## 实现摘要

后端新增只读接口 `GET /api/v1/sales/items`（`server/internal/sale/item_query.go`、`item_query_repository.go`）：

- 请求沿用单据分页的筛选条件，另加 `sortBy`（`businessDate`/`unitPrice`/`amount`/`netAmount`）与 `sortDir`；排序字段与方向在 Service 白名单校验，用户文本不进入 SQL。
- 每条记录对应一条原销售明细，带来源销售身份与单号、客户身份与已保存名称、业务日期、状态、商品身份与快照描述、类型与单位、数量、成交单价、原金额、有效退货数量与金额、净数量与净额及适用性。
- 有效退货按原销售明细先聚合仍已过账的销售退货，再与销售关联，多笔退货不会倍增原数量、原金额或总条数；退货金额取已保存行金额。
- 返回 `records`、`total`、`page`、`pageSize` 与同筛选范围的 `summary`：金额按 `DRAFT`/`POSTED`/`CANCELLED` 分别列出原金额，已过账另有有效退货与净销售额；数量按商品身份、单位与状态分组。汇总不受页码、每页条数和排序影响，没有匹配时记录为空、金额为零、数量分组为空。
- 计数、明细分页与两类汇总在同一个可重复读只读事务内完成（SQLite 与 PostgreSQL 都支持），并发过账、退货或取消不会让同一次查询读出互相矛盾的数据。
- 未新增表、索引或迁移；查询使用两库公共 SQL 子集。

前端只改销售出库页面（`web/src/pages/business/sales.tsx`）与显式选择的公共能力：

- 筛选字段占满面板宽度，查询与重置在面板底部靠右，刷新移到列表工具栏；新增本月与已过账快捷条件。
- 默认按单据：单据行直接显示销售内容摘要（可展开整单全部明细），商品筛选时突出匹配商品行。
- 可切换到按明细：跨单展示销售对象、数量与单位、成交价、原金额、有效退货与净销售额，并展示全结果的状态分组金额合计与按商品身份/单位的数量合计；支持单价、原金额、净销售额排序与 10/20/50 每页。
- `SearchFilterBar` 增加显式选择的 `stacked`/`leadingActions`，`DataTable` 增加可选展开行，`useDocumentSearch` 的 `apply` 增加可选的一并重置参数，`lib/datetime` 增加 `businessMonthRange`；默认行为都不变，其他页面不需要改动。
- 加载失败保留条件并提供重试；过期响应不覆盖更新的查询；查看原单后返回保留视图、筛选、排序与页码。

## 实现中的判断项

- 数量分组的商品描述取该分组自身已保存的快照（编码、名称最小者），改名不会给历史销售重新贴标签，也不会把同一商品身份拆成两组。
- 净销售与有效退货只统计已过账销售；草稿与已取消销售保留原数量与金额，净值标记为不适用；服务不能退货，净数量与净额等于原值。
- 净额排序把净值不适用的行稳定排在有效行之后，升序降序一致。
- 重置清空查询条件与页码，保留当前视图与排序。

## 实际执行的检查

- `TestSQLiteSaleItemQueryHTTPContract`、`TestPostgresSaleItemQueryHTTPContract`：同一套断言在 SQLite 与 PostgreSQL 上实际执行通过，覆盖整单与混合/纯服务明细、同商品不同价格、零价、草稿、已过账、已取消、多次部分退货、跨月退货、草稿退货、退货取消、服务不适用、商品与往来关键词逐明细匹配、商品/客户改名与停用的历史快照、排序、分页稳定、汇总不随页码与每页条数变化、空结果与非法参数。Taskfile 与 CI 的现有选择器分别各选中一次。
- `task db:check`：`task backend:check`（`go test ./...`、`go vet ./...`）与 SQLite 集成契约通过；PostgreSQL 套件只在 `TestPostgresPartnerAttachmentBackupRestore` 失败，原因是宿主机未安装 `pg_dump`（`exec: "pg_dump": executable file not found in $PATH`），与本次改动无关，其余 PostgreSQL 契约全部通过。
- 针对最终代码单独复跑：SQLite 集成契约 `go test -tags=integration ./integration -run '^TestSQLite'`（27.6 秒）与 `-run '^TestPostgresSaleItemQueryHTTPContract$'`（5.5 秒）实际通过。
- `task frontend:lint`、`task frontend:build` 通过。
- `node tests/datetime.mjs`：新增 `businessMonthRange` 在 UTC、America/Los_Angeles、Asia/Shanghai 三个宿主时区下的断言，实际执行通过。
- `node tests/issue66-sale-query.mjs`：隔离 SQLite API + 真实 Chromium，实际执行通过；复现入口见 [web/tests/issue66-sale-query.md](../web/tests/issue66-sale-query.md)。
- `task api:docs`：Swagger 重新生成，差异只有新接口、其说明与新增定义，没有删除既有内容。

## 未执行项与已知问题

- 经营者试用未进行；自动检查不替代用户查看销售页面实际效果。
- 未推送代码、未关闭 GitHub issue。
- `web/tests/phase5-document-query.mjs` 在 main 上即已失败（与本次改动无关）：其 `select()` 依赖 `getByRole('textbox')` 与 `button[aria-pressed]`，而现有档案选择器的搜索框是 `role="combobox"`、候选项是 `role="option"`；失败点在未改动的采购页面，本次未修改该脚本。销售页面的单据查询行为改由 `tests/issue66-sale-query.mjs` 覆盖。
- 共用搜索钩子的既有竞态：选择档案后立刻填写业务日期并查询，日期条件可能未生效（实测等列表刷新后再填写即正常）。属 `useDocumentSearch` 既有行为，采购页面同样适用，本次未修改。
