# 独立直送单与采购来源实施记录

## 发布与范围

规格 [#78](https://github.com/EziosWJ/simple-inventory/issues/78)，任务 [#79](https://github.com/EziosWJ/simple-inventory/issues/79)～[#83](https://github.com/EziosWJ/simple-inventory/issues/83) 已按用户授权发布，均带 `ready-for-agent` 标签。任务依赖为 #80 ← #79、#81 ← #80、#83 ← #81/#82；#82 可独立开始。父规格保持打开。

已完成 T01～T05 的实现与统一验收：独立直送草稿、一次确认同时生成采购销售、整笔取消及旧版兼容、普通销售多采购来源、双库升级及浏览器验收。代码变更按本次本地提交保存；未推送或部署，GitHub 子任务仍保持打开；日常业务数据库未运行迁移。

## 已交付行为

- 独立直送单选择一个供应商和客户；实物共享数量，采购价与销售价分别录入，服务只进入销售侧。草稿不影响库存和往来。
- 确认时在一个事务内创建并过账采购单和销售单；库存保留成对直送流水、净变化为零，应付与应收各自记账。失败回滚，重试不会重复生成。子单可查看、打印、退货和追溯，但不能独立编辑、过账或取消。
- 从直送单入口整笔取消对应采购和销售，保留原单、反向流水和实际收付款；有效退货会阻止取消。取消完成后，经营者可按实际情况独立新建直送单，双方没有重开关系或自动继承。
- 店内销售退货和后续供应商采购退货继续走现有流程。客户送货单复用销售送货单，不展示采购价。
- 旧版直送及已有草稿保留原规则和操作入口；新采购不再提供旧直送勾选，新普通销售使用采购来源。旧来源 URL 仍可完成遗留草稿流程。
- 普通销售每条商品明细可选多个相同商品的已过账采购明细，记录来源数量。来源可选、可部分记录，不占用采购数量，也不限制多个销售重复引用；同一销售明细来源合计不能超过销售数量。晚于销售日期时需明确确认。
- 过账后可修改来源并保存操作人、时间及前后值；不改变销售、库存或往来。采购后续取消或退货时保留来源并展示状态及退货数量。服务行不记录来源；来源编辑不适用于新直送子单和旧版直送销售。
- 双库 schema 从 27 升至 29，seed 25；直送草稿、生命周期和销售来源各自增量迁移，升级保留既有旧版数据。

## 统一验收记录

### 已通过

- 后端 `task backend:check`（含 Go tests 与 vet）。
- SQLite 全量集成契约，在本轮 `task db:check` 中通过。
- 新增直送生命周期、采购来源及升级契约在 SQLite 与 PostgreSQL 均定向通过，覆盖失败回滚、并发与重试、版本和权限、双向退货、来源审计、旧数据升级。
- 完整 `task db:check` 通过：后端检查、SQLite 全量契约（31.961 秒）、PostgreSQL 全量契约（340.886 秒）；Postgres 备份恢复契约使用临时 `pg_dump`/`psql` 包装器，在契约启动的隔离 Docker PostgreSQL 17 容器内完成，主机无需安装客户端。
- 固定 `task api:docs` 生成成功，Swagger 源注解及生成结果一致。
- `task frontend:lint`、`task frontend:build` 通过；构建只有原有 Browserslist 数据和大 chunk 提示。
- 新增真实浏览器脚本通过，覆盖直送确认、取消、未知结果核实、退货阻止取消、旧版来源入口、多采购来源、晚日期确认、过账后来源编辑审计及已取消采购来源保留。
- 既有 `direct-order-drafts.mjs` 与 `phase5-sale-post.mjs` 浏览器回归通过；销售过账兼容脚本已更新过时的选择器与库存提示断言。
- `git diff --check` 通过。

### 验收边界与日志

首次完整门禁发现四个 Postgres 升级夹具复用了迁移前的 `SELECT *` 查询计划；升级后重置空闲连接池以模拟 API 重启，四个用例已在两个数据库分别定向复验。第二次完整门禁仅因调用命令时误用了临时 pg_dump 包装器目录而在备份恢复用例失败；使用正确的 `/tmp/direct-drafts-pg-tools` 路径重新运行完整门禁后全部通过。最终日志：`/tmp/direct-redesign-db-check-verified.log`；首次日志为 `/tmp/direct-redesign-db-check-final.log`；升级定向日志为 `/tmp/direct-redesign-upgrade-sqlite.log`、`/tmp/direct-redesign-upgrade-postgres.log`；备份恢复单项补验为 `/tmp/direct-redesign-backup-restore.log`。

固定 Swagger、前端和浏览器日志分别为 `/tmp/direct-redesign-swagger.log`、`/tmp/direct-redesign-lint-final.log`、`/tmp/direct-redesign-build.log`、`/tmp/direct-redesign-browser.log`、`/tmp/direct-redesign-legacy-browser.log`、`/tmp/direct-redesign-sale-post-browser.log`。更广的 `sales-query-fixes.mjs` 脚本尝试运行，但在第 142 行等待商品筛选 URL 更新时超时，尚未执行其中的过账预览断言；本次改动相关的预览行为由通过的 `phase5-sale-post.mjs` 核对。打印模板没有修改，送货单页面由真实浏览器路径验收；未重跑独立 PDF 全量验收。

## GitHub 任务状态

代码提交仅保存在本地，尚未推送或部署。按照用户授权直接发布过规格和任务；GitHub #79～#83 保持打开，父规格 #78 也保持打开。
