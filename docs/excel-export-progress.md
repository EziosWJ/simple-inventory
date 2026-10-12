# Excel 明细导出交付记录

日期：2026-10-12。父规格 [#85](https://github.com/EziosWJ/simple-inventory/issues/85)，任务 [#86～#89](excel-export-tickets/README.md)。

本地代码已完成实现及验收；本次未提交、推送、部署或关闭 GitHub issue。开始工作时已有的 `CONTEXT.md` 修改、导出规格及任务文档、`.claude/worktrees/` 内容已保留。

## 已实现

- #86：采购明细 Excel，两个列表视图均有导出入口；共享 `platform/xlsx` 写入包、前端 `useExcelExport` 流程和文件响应通道。
- #87：销售明细 Excel，沿用当前明细排序，服务退货与无效净额留空。
- #88：往来明细 Excel，完整期间的单单位模式展示期间余额，关键词及不完整期间模式按单位、方向汇总；生效终点按不包含口径取文件日期。
- #89：三页一致性、数据不变性、双库和浏览器回归验收。

采购和销售导出复用列表查询构造器，在一个可重复读只读事务内读取总行数、所有明细及两个合计；往来导出复用既有明细与期间余额查询，在同一只读事务内组装。往来取数最多读取上限加一行以拒绝超限，不分页拼接。导出人使用当前认证用户的昵称，空昵称回退用户名。

三个文件统一为「明细」「合计」两个 sheet。金额、数量使用十进制数值单元格；日期和时点使用文本，业务时间按 Asia/Shanghai 展示；空结果仍含表头与零合计。明细末行只填原金额／变动合计与行数／笔数。文件流保留既有 JSON 错误信封；中文文件名使用 `filename*=UTF-8''`，同时向跨域前端暴露 Content-Disposition。

三页导出使用点击时查询框的内容；预检与下载共用相同的条件副本。大于 10000 行先确认，取消不请求导出；下载及预检期间禁用按钮。后端大于 100000 行拒绝，并返回规格指定的中文提示。导出不写审计记录、不改变业务数据，认证及页面权限沿用现有查询约束。

## 验证结果

以下检查通过：

- `task backend:check`：Go 测试及 vet。
- `task api:docs`：使用 Taskfile 固定命令重新生成。对 Swagger JSON 与 HEAD 做结构比较：只新增三个 `/export` 路径，已有路径与 definitions 等其他结构无变化。
- `task frontend:lint`、`task frontend:build`。build 保留既有 Browserslist 数据过期与 chunk 大小提示。
- 定向 SQLite 与 PostgreSQL 契约各一批：

```sh
task db:contract -- '^TestSQLite(ExcelExportHTTPContract|SaleItemQueryHTTPContract|MasterDataSelectionHTTPContract|PartnerStatementPostgresSQLiteHTTPContract)$'
task db:contract -- '^TestPostgres(ExcelExportHTTPContract|SaleItemQueryHTTPContract|MasterDataSelectionHTTPContract|PartnerStatementPostgresSQLiteHTTPContract)$'
```

新增 `ExcelExportHTTPContract` 通过真实 HTTP 读取工作簿，覆盖：17／18／12 列、两个 sheet、数值／文本类型、历史型号规格、仍有效的退货、服务和非过账净额留空、全部排序字段与方向对照列表、交付方式及商品筛选、空结果、参数错误、中文文件名、跨域文件名响应头、往来期间及多单位分组、冲销来源、部分期间、真实超限拒绝和未认证拒绝。对导出前后采购、销售、库存、往来及操作审计表进行完整内容比较。

新增契约名分别为 `TestSQLiteExcelExportHTTPContract`、`TestPostgresExcelExportHTTPContract`，已核对 Taskfile 与 CI 的两个正则分别选中对应数据库用例一次。

现有真实浏览器回归通过，均对 `/tmp/si-export-ui/acceptance.db` 隔离 API 运行：

```sh
PHASE5_API_URL=http://127.0.0.1:18285 node web/tests/phase5-document-query.mjs
PHASE5_API_URL=http://127.0.0.1:18285 node web/tests/issue66-sale-query.mjs
PHASE5_API_URL=http://127.0.0.1:18285 node web/tests/sales-query-fixes.mjs
```

这些脚本保留了精确名称的「按单据」「按明细」按钮匹配。同步修复已有脚本的两个过时假设：选择／清除档案后点击查询才生效；同一单号在明细视图可有多行，切回单据时要等到带单据操作的行出现。状态下拉框按状态选项定位，避免误选交付方式。

另用临时浏览器脚本验收三页真实下载、中文文件名、采购销售两个视图、当前尚未提交的查询框条件和往来部分期间。通过临时拦截大行数预检响应及错误响应验证确认／取消、禁用及失败恢复；实际工作簿仍由隔离 API 下载，真实 100000 行上限由双库 HTTP 契约验证。未新增 `web/tests/*.mjs`。

完整日志位于 `/tmp/si-export-backend.log`、`/tmp/si-export-swagger.log`、`/tmp/si-export-lint.log`、`/tmp/si-export-build.log`、`/tmp/si-export-sqlite-final.log`、`/tmp/si-export-postgres-final.log` 与 `/tmp/si-export-ui/*.log`。

未执行全量 `task db:check`：本批是三个局部查询导出，未修改 schema、migration 或公共数据库／锁机制，按根目录验证分层使用受影响契约定向双库检查。未执行发布构建和正式部署。
