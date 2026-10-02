# Phase 5 自动验收复现

覆盖 T01–T08 的真实浏览器业务及 T09–T11 的隔离 HTTPS/恢复。最终结果见 `docs/phase-5-acceptance.md`，父母实际试用另用 `docs/phase-5-trial.md` 记录，未执行不视为通过。

使用隔离数据库和文件目录；脚本会真实创建资料、草稿并过账，不对日常业务库执行。脚本默认管理员账号 `admin`，密码通过 `PHASE5_UI_PASSWORD` 指定，未指定时使用开发种子密码。

从仓库根目录迁移并启动独立 SQLite API：

```sh
APP_DATABASE__URL=/tmp/phase5-acceptance.db task db:migrate:sqlite
APP_DATABASE__URL=/tmp/phase5-acceptance.db \
APP_HTTP__ADDRESS=127.0.0.1:18109 \
APP_FILE__STORAGE_ROOT=/tmp/phase5-acceptance-files task api:sqlite
```

从 `web/` 执行，下列脚本依次使用 4189–4196 端口，自行启动 Vite 并在结束时清理：

```sh
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-purchase-search.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-purchase-drafts.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-purchase-post.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-sale-drafts.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-sale-post.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-document-query.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-partner-query.mjs
PHASE5_API_URL=http://127.0.0.1:18109 node tests/phase5-workbench-accounts.mjs
```

需要安装与 Playwright 对应的 Chromium，或使用 `PLAYWRIGHT_EXECUTABLE_PATH` 指定已有可执行文件。

- 搜索脚本通过公开 API 创建超过 500 条商品和供应商，检查分页、辨认信息、类型/身份/启用过滤、字面通配符、大小写、保留选择、迟到响应、空结果、重试、键盘、保存编辑及过账。
- 草稿脚本检查独立路由/刷新、整数金额与同商品分行、未保存内部离开/原生刷新提醒、服务端成功但新建/编辑响应丢失、另一个真实账号引起的版本冲突及后续过账、最小恢复标识、未提交标识关闭和迟到请求拒绝、终态禁止编辑。
- 过账脚本检查确认取消、真实保存失败不继续、真实资料校验失败留同一草稿、响应丢失核实已生效、未到达服务端后的刷新/核实/同版本重试、重复点击、另一账号取消或改版本后停止。
- 销售草稿/过账覆盖超过 500 条商品/客户、混合实物/服务、纯服务、稳定明细 ID、默认及明确空送货字段、直送来源、库存不足修改原草稿重试、分行求和和零金额；确认不会制造第二张单据。
- 历史单据检查组合筛选、日期/页码刷新、历史身份移除/停用、明细重复不重复计数、详情编辑返回原条件和迟到响应。往来脚本检查两个方向的负余额、完整期间分页、全部来源及冲销、返回原范围和真实三页 PDF。
- 首页脚本创建真实业务菜单角色和两名用户，实际登录完成共同录单/查询、冲突保留输入、实际创建人/过账人、菜单搜索和通知管理 403；未用 mock 权限。
- 两类后端公开 HTTP 契约和增量迁移契约使用 `TestSQLitePhase5…` 与 `TestPostgresPhase5…` 前缀，现有 Taskfile/CI 分别选中一次。保存契约关闭并重新打开实际数据库检查持久回执。

验收入口：`task check`、`task db:check`、`task build:check`。API 注解修改后运行 `task api:docs`。

采购保存结果有三类：`COMMITTED` 返回原成功回执和单据最新状态；`UNCONFIRMED` 不证明失败；显式核实动作在等待原事务后，如确未提交则关闭原键并返回 `NOT_COMMITTED`，防止迟到请求再次创建。成功保存、明细、回执与成功审计使用同一个事务。

销售遵守同一结果核实约定，保持业务自己的回执作用域；省略送货字段与明确空值、原明细 ID 及关联采购也纳入指纹。

## 生产与恢复复现

按 `deploy/production/README.md` 准备隔离 Compose、私有配置、显式迁移和账号。`phase5-production.mjs` 不启动 Vite，直接验证已运行的 HTTPS 内嵌前端。私有 `PHASE5_PRODUCTION_FIXTURE` 为 mode-600 JSON，包含 `isolated: true`、两名已初始化账号的 `operators: [{username,nickname,password},…]`。`seed` 通过 UI 创建业务并保存测试 ID/原结果；`check` 核对重建后同一批结果；`write` 在新恢复项目核对原结果后过账一笔销售；`after-write` 核对已完成写入的零库存/新增应收。不要对正式业务运行。

```sh
PHASE5_PRODUCTION_FIXTURE=/private/isolated-fixture.json \
PHASE5_PRODUCTION_URL=https://localhost:18443 \
PHASE5_PRODUCTION_MODE=seed node tests/phase5-production.mjs
```

旧版回退的 UI 使用 `PHASE5_PRODUCTION_LEGACY=1`。API 证书另用显式 CA 验证；浏览器脚本忽略 HTTPS 错误仅限 `isolated: true` 的本地 CA 演练，不宣称正式证书有效。恢复后的文件按 SHA-256 比较，原单据按公开 HTTP 全字段比较，并读取数据库核对审计和流水累计。

`python3 deploy/production/test-backups.py --env <隔离私有配置> --directory <新隔离目录>` 仅允许 `phase5-*` 项目，实际暂停业务并测试备份、重叠、归档中途失败/服务恢复、30 天清理归属和损坏恢复拒绝；仅在无其他验收写入时执行。每日任务由 `schedule-backup.py` 安装，使用同一个备份入口，演练结束移除该项目的 cron 项。

完整送货单及对账 PDF 使用 `phase4-acceptance.mjs` 和 `phase4-print-pdf.py`；变量见 `phase4-acceptance.md`。生产内嵌测试将 API/WEB 地址都设为该隔离 HTTPS 地址，只有本地 CA 时设置 `ACCEPTANCE_ISOLATED_LOCAL_CA=1`。PDF 核对三页送货单（金额显隐、草稿、旧 NULL 快照）和四页完整期间对账（冲销、末页合计），不是仅查看 DOM 预览。
