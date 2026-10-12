# 销售出库整改交付记录

日期：2026-10-12（含 T06 页面反馈复验）。父规格：[GitHub #66](https://github.com/EziosWJ/simple-inventory/issues/66)。交付范围为 T01～T07（#67～#73），仅使用隔离 SQLite 数据库和独立 PostgreSQL 契约数据库。

## 实现与验收对应

| 任务 | 实现 | 验收证据 |
| --- | --- | --- |
| T01 查询状态 | 分离详情、单据结果、明细结果与合计展示；已应用条件保持稳定，编辑输入只随筛选变化或显式应用同步；详情按销售 ID 独立读取 | 未提交单号/日期在翻页、排序、切换视图与详情后保留；快速选择后立即填日期；详情开关不增加列表请求；浏览器历史与编辑返回 |
| T02 反馈与页码 | 统一日期校验；详情有独立加载、失败、重试；合计加载时隐藏数字，失败明确不可用；越界页按真实 total 校正 URL 并重查 | 非法日期及快捷选择不发送无效查询；详情失败重试和迟到响应；列表失败与合计不可用；真实取消记录后校正末页、旧页和空结果 |
| T03 内置浏览 | 销售客户/商品输入尾部有可聚焦、具名称及说明的浏览图标；清除按钮独立；其他调用方保留默认布局 | 两个入口同排、输入和键盘提交、快速选择、完整面板、历史停用商品、清除再输入；共用选择器另验分页与录单行为 |
| T04 匹配标识 | 服务端 `SaleLine.matchesProduct` 标识真正匹配行；整单选择、逐行标记、明细筛选共用同一 SQL 谓词；前端只消费标识 | 定向双库 HTTP 契约验证 ID、历史描述、当前改名/品牌/型号、多词跨当前档案与快照、大小写、字面 `%`/`_`；整单保留全部行，明细与标记一致 |
| T05 过账影响 | 列表过账及录单确认复用库存提示；按商品 ID + 单位归集，以 bigint 千分数量计算，完整显示编码、名称、型号规格和单位 | 同名不同规格与单位，0.1 + 0.2 = 0.3，零价实物、混合服务、纯服务；实际库存核对为 99.2 支、87.875 盒；录单异常、重试和直送另回归 |
| T06 结果可读性 | 净销售额及退货以金额为主、取消负零；来源单号可复制并保留完整 title；三种状态用独立 panel，桌面等宽一行、窄屏堆叠；数量合计在 modal 查看，窄屏用分组卡片 | 桌面/窄屏真实浏览器检查；长客户名、服务、不同状态与完整数量分组；数量归集保留 0.8 支；分行成交价格分别展示 |
| T07 整体验收 | 保留全结果口径及原销售详情、录单、送货单流程；提供独立数据、复跑入口、截图与检查记录 | 下述浏览器、双库、API 文档及前端检查；经营者试用单独记录；T06 新布局复验通过 |

## 检查结果

- `task frontend:lint`：通过，无错误或警告。
- `task frontend:build`：通过。仍有既有 Browserslist 数据陈旧及大 chunk 提示。
- `task api:docs`：通过，使用 Taskfile 固定工具版本。生成差异仅为 `sale.Line.matchesProduct` 及销售分页源注解描述。
- `task db:contract -- '^TestSQLiteSaleItemQueryHTTPContract$'`：通过，实际执行。
- `task db:contract -- '^TestPostgresSaleItemQueryHTTPContract$'`：通过，实际执行。
- `task db:check`：已完整运行一次；后端测试与 vet、SQLite 全量通过。PostgreSQL 全量唯一失败是 `TestPostgresPartnerAttachmentBackupRestore` 缺少本机 `pg_dump`，其余检查通过。
- 为上述唯一失败项提供 PostgreSQL 17 测试容器中的真实 `pg_dump` / `psql`，再定向运行 `TestPostgresPartnerAttachmentBackupRestore`：通过。客户端在对应隔离数据库容器内执行，真实导出、导入与附件恢复断言没有替换或跳过。其余已通过检查复用，未重复整套门禁；首次 `task db:check` 的退出码仍记录为失败。
- 五项浏览器检查均通过，按本批次分批执行并复用已通过结果：`issue66-sale-query` 与 `sales-query-fixes` 日志位于 `/tmp/sales-query-fixes.ph1wlK/`；`phase5-document-query` 与 `phase5-sale-post` 日志位于 `/tmp/sales-query-fixes.3jDBpq/`；`master-data-selection` 日志位于 `/tmp/sales-query-fixes.RXHWcI/`。各目录内日志文件名为对应脚本名加 `.log`。早期整套脚本运行中的失败保留在日志中，以上结论来自各项最终实际通过的执行。
- `git diff --check`：通过。
- 2026-10-12 T06 反馈复验：`task frontend:lint`、`task frontend:build` 均通过；隔离 SQLite 浏览器脚本 `issue66-sale-query` 与 `sales-query-fixes` 均通过。检查了 1440px 桌面三卡同排、390px 窄屏三卡堆叠、桌面数量表格 modal 和窄屏数量卡片 modal。
- 本次复验日志位于 `/tmp/sales-summary-redesign-{lint,build,browser}.log`；浏览器截图原始目录为 `/tmp/sales-query-fixes.fmPakn/screenshots/`，代表截图已归档到下方链接。

主要日志保留于 `/tmp/sales-fixes-{lint,build,api-docs,sqlite,postgres,db-check,backup-contract,acceptance,regressions,selection}.log`。初次沙箱内 PostgreSQL 用例因 Docker 不可访问而跳过，未计为通过；上面的 PostgreSQL 结论来自沙箱外的实际执行。

## 复跑

从仓库根目录执行：

```sh
bash web/tests/sales-query-fixes.sh
```

脚本创建新的 `/tmp/sales-query-fixes.XXXXXX/acceptance.db`，使用 `task db:migrate:sqlite` 与 `task api:sqlite`，依次运行：

1. `web/tests/issue66-sale-query.mjs`：客户/商品双路径、跨期有效退货、状态合计、排序分页、迟到列表响应与详情上下文。
2. `web/tests/sales-query-fixes.mjs`：本轮状态隔离、内置选择、品牌匹配、错误/加载/重试、迟到详情、精确过账、库存持久状态、页码校正与截图。
3. `web/tests/master-data-selection.mjs`：共享选择器默认布局、键盘、分页、停用资料、参考价及录单衔接。
4. `web/tests/phase5-document-query.mjs`：采购及销售的历史资料、日期组合、分页、详情/编辑返回和迟到响应。
5. `web/tests/phase5-sale-post.mjs`：销售保存/过账失败、丢失响应、安全重试、版本冲突、零金额、服务、直送及送货单。

结束时清理自身 API 进程组，保留数据库、日志与截图。默认 API 端口 18179，可设置 `SALES_API_PORT`；需要 Node 依赖、Go、Task、Playwright Chromium、curl、setsid。可通过 `PLAYWRIGHT_EXECUTABLE_PATH` 指定 Chromium。脚本使用独立测试管理员，禁止把它指向正式数据库。

迭代时可用 `SALES_ACCEPTANCE_SCRIPTS` 选择需要复跑的脚本，例如：

```sh
SALES_ACCEPTANCE_SCRIPTS='phase5-document-query phase5-sale-post' bash web/tests/sales-query-fixes.sh
```

数据库与 API 文档检查沿用根 Taskfile。PostgreSQL 备份恢复用例需要可用的 `pg_dump` / `psql`；本轮临时容器客户端位于 `/tmp/sales-fixes-pg-tools/`，补验命令：

```sh
PATH=/tmp/sales-fixes-pg-tools:$PATH task db:contract -- '^TestPostgresPartnerAttachmentBackupRestore$'
```

## 页面效果

代表截图使用真实页面和公开 API 创建的隔离业务数据：

- [单据匹配展开](screenshots/documents.png)
- [紧凑明细合计](screenshots/items-compact.png)
- [桌面数量合计弹窗](screenshots/items-quantities.png)
- [窄屏数量合计弹窗](screenshots/items-quantities-mobile.png)

## 经营者试用与未执行项

经营者人工试用尚未进行；以上全部为自动化浏览器与 HTTP 检查，不能作为人工反馈。后续请分别记录客户期间查询、商品历史价格查询及过账核对的经营者反馈。

本轮没有 schema/migration 改动，无新增空库/旧版本升级任务。全量门禁中的既有迁移契约已执行。没有执行正式服务器部署、数据迁移或生产账号操作。GitHub Issue #67～#73 已按验收完成关闭；父规格 #66 保持原样。
