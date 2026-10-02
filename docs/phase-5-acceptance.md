# Phase 5 本地技术验收记录

2026-10-02：T01–T11 本地实现及技术验证完成，T12 自动验收和试用材料准备完成，**父母实际试用待人工**。代码/文档尚未提交或推送，GitHub #40–#51 尚未关闭；没有操作正式服务器、DNS、公开证书或真实店铺数据。本记录不宣称阶段已适合正式启用。

## 实现与验证

| 任务 | 交付行为 | 实际验证 |
| --- | --- | --- |
| T01 #40 | 商品/供应商字面分页搜索，按 ID 保留选择，辨认型号/联系方式 | 双库公开 HTTP；真实 UI 各超过 500 条、同名、停用、类型/身份、特殊字符/ASCII 大小写、迟到/错误/空结果及键盘 |
| T02 #41 | 采购完整页、可靠草稿、原成功回执及显式核实、最小恢复标识、离开/版本保护 | 双库并发/重试/事务回滚/审计、真实重开 DB、23→24 保留；UI 新建/编辑丢响应、双账号竞争及刷新 |
| T03 #42 | 采购保存后只过账原 ID/版本，确认/真实失败留原草稿，未知结果核实 | UI 取消无写、保存失败无过账、丢响应/未到达/重复点击、第二人改版/取消停止；既有采购/直送/零金额契约 |
| T04 #43 | 销售完整页、稳定明细 ID、服务/实物、送货默认/明确空值、可靠回执及直送 | 双库并发/规范化/指纹冲突/审计回滚/重开 DB、24→25 保留；UI 超过 500 条、混合/服务/同商品分行、打印及安全核实 |
| T05 #44 | 销售整单确认、原回执过账、库存不足同草稿编辑重试 | UI 真实不足/停用、分行合计/零金额/纯服务、丢保存/过账响应、第二账号、直送先采购后同销售重试 |
| T06 #45 | 单据组合查询/分页 URL、历史对象、来源及编辑返回原范围 | 双库日期/状态/对象/商品/多行计数/快照 NULL、字面单号；UI 刷新/返回/重置/迟到响应 |
| T07 #46 | 正负双向余额、实际生效期间完整往来、来源/冲销及打印返回 | 双库期间/退款/期初/实际时间契约；UI 两方向待退款、原身份移除/停用、全部来源追溯和三页 A4 对账 |
| T08 #47 | 按真实菜单授权的四类首页入口，两人共享店铺业务 | 两个实际角色/独立登录，草稿接力、版本冲突保留输入、真实创建人/过账人、余额/来源、菜单搜索、通知管理实际 403 |
| T09 #48 | 版本镜像、内部网络/可信代理、生产内嵌 HTTPS、显式迁移及非 root 持久卷 | 空库只读检查失败、不建迁移表；显式迁移、失败续跑；真实证书 CA 校验、UI 深链接/刷新/共享业务、Swagger 404、重建同单/文件 SHA-256 保留 |
| T10 #49 | 暂停写入的一致整套备份、互斥/原子成功、30 天保留、每日 cron、异地复制说明 | 真 pg_dump/上传/config/镜像/迁移/校验；中途归档失败非零并恢复服务，重叠拒绝，完整/不明/损坏/他项目清理范围；实际安装 cron、核对 cron 服务 active 后移除隔离任务 |
| T11 #50 | 新项目/新卷整套恢复、匹配旧应用回退及升级失败处理 | 当前 schema 25 恢复 23 张业务/文件/流水/审计/迁移表逐表一致；旧业务 23→25 保留原记录、旧守卫拒绝新 schema，再恢复发布前 23 + 旧镜像；两种恢复均实际读取文件/PDF及销售写入，库存/往来累计和审计一致 |
| T12 #51 | 完整门禁、生产浏览器/PDF矩阵、父母代表性试用及记录模板 | 自动项通过；人工清单在 `phase-5-trial.md`，两人结果均待试用 |

采购/销售保存请求按用户/操作分区，回执、业务、明细和成功审计同事务。查询无记录不证明失败；显式核实等待原事务后可关闭原键，防止迟到请求新建。完整页只持久化最小恢复标识，不自动保存完整未提交表单。

经营者使用原有菜单角色。验证不假定全部系统接口隔离：现有用户/RBAC 管理写接口只要求登录，通知管理要求管理员。阶段保留可信两名经营者的现有访问范围，未引入新的岗位权限模型。

## 门禁与打印

- 最终 `task check`、`task build:check` 均退出 0；Go tests/vet、前端 lint/build 和生产内嵌构建通过。
- 完整 `task db:check` 退出 0：SQLite 29.720 秒、隔离 Docker PostgreSQL 282.199 秒，包含 backend tests/vet。新增 `TestSQLitePhase5…` 与 `TestPostgresPhase5…` 被 Taskfile/CI 的原选择器各选中一次，数据库契约没有因漏标签而被跳过。
- 新 schema 24/25 两库空库及旧数据升级通过，逻辑版本 25、seed/menu 既有断言同步；Goose 是唯一 schema 入口。保存回执在关闭/重开真实 DB 后仍可查询。
- REST 源注解同步，按固定 `task api:docs`（swag v1.8.12）重新生成；没有手工编辑 generated 定义。
- 最终八个 Phase 5 浏览器脚本及 Phase 4 real-ui/fixes 通过。完整 Phase 4 浏览器在最终 HTTPS 内嵌前端通过 11 个阶段，未使用 Vite 或 mock API。
- 实际 PDF：显示金额草稿、隐藏金额草稿、历史过账快照送货单各 3 页；含 43 条完整期间和 3 次冲销的对账单 4 页。Poppler 核对真实 A4、页数、每页页码、全部文字边界、末页签收/合计；末页栅格已目视检查。历史送货型号/规格 NULL 在档案补值后仍保持空白。对账期末 370.00，送货总金额 820.00。
- Bash/Python/Node 语法检查及 `git diff --check` 通过。前端 build 有原有 chunk 大小/Browserslist 提醒，未作为失败，也未为本阶段扩展拆包范围。

## 隔离发布和恢复版本

最终候选发布 `phase5-final-20261002`，schema 25：

```text
API     sha256:8a7af96ccc6998711da04d6fcd15cd7332319b19e6063bd2487682ac59f85644
migrate sha256:0a09ad5ab102a16a649faaa28465cf08ee0a4e2257978bc18ab4211e29df5ce5
PostgreSQL / pg_dump 17.10
Caddy 2.11.4
API uid=100(app), gid=101(app)
```

旧业务基线为 Git `e7f2948e14fe94081af8b7a4f2de399296375557`（schema 23），演练发布 `phase4-rollback-20261002`。为配合新部署包，只回植只读迁移检查和 Docker 非 root 文件归属；旧业务实现/前端取基线，未让旧业务二进制读取升级后的 schema 25。发布前备份恢复到独立项目后，原业务/流水/快照/审计/schema 23 逐表一致。

恢复后各过账一笔销售：目标库存从 1 到 0、客户应收从 300 到 600 分，新增且仅新增 CREATE/POST 两条审计；库存余额等于库存流水求和，往来余额等于同对象/方向往来流水求和。当前恢复还比对全部 23 张业务/文件/流水/审计/迁移表；不是只列出备份文件或 `pg_restore --list`。

损坏备份在建目标前拒绝；校验完整但迁移元数据错误的备份导入全新库后明确拒绝启动 API；原环境未覆盖。恢复 API 启动守卫检查所有预期版本及未知新版本，不隐式迁移。

## 证据与复现

复现入口为 `web/tests/phase5-acceptance.md` 和 `deploy/production/README.md`，隔离项目/端口使用独立配置。凭据和原始证据未提交 Git。

| 证据 | 本机位置 |
| --- | --- |
| 最终代码/内嵌/双库门禁 | `/tmp/phase5-delivery-final-check.log`、`/tmp/phase5-delivery-final-build.log`、`/tmp/phase5-final-db-check.log` |
| Phase 5 浏览器 | `/tmp/phase5-final-*.log`、`/tmp/phase5-delivery-workbench.log`、`/tmp/phase5-delivery-partner-query.log` |
| 完整 HTTPS 浏览器及真实 PDF | `.task/phase5-production/full-acceptance/browser-result.json`、`pdf-result.json`、PDF 和末页 PNG |
| 生产构建/初始化/重建 | `/tmp/phase5-production-final-images.log`、`/tmp/phase5-production-final-start.log`、`/tmp/phase5-production-recreate-browser.log` |
| 备份成功/失败/锁/保留/运行版本 | `/tmp/phase5-backup-contract-final.log`、`/tmp/phase5-backup-release-mismatch.log`、`.task/phase5-production/backup-contract-final/`（机密，mode 700/600） |
| cron 安装/移除 | `/tmp/phase5-backup-schedule.log`、`/tmp/phase5-backup-schedule-remove.log` |
| 当前整套恢复/写入/PDF | `/tmp/phase5-current-restore.log`、`/tmp/phase5-restored-current-browser.log`、`/tmp/phase5-restored-current-delivery.pdf`、`.task/phase5-production/restore-result.json` |
| 真实升级、旧版拒绝新库、旧版回退 | `/tmp/phase5-upgrade-start.log`、`/tmp/phase5-old-new-schema-rejected.log`、`/tmp/phase5-rollback-restore.log`、`/tmp/phase5-rollback-browser.log` |
| 元数据不匹配备份拒绝 | `/tmp/phase5-mismatched-restore.log` |

调试发现并已修正镜像复制的 600 迁移文件归属、固定代理地址被 API 自动占用、非默认 HTTPS 端口 Origin 和直送完整页返回文案。验收脚本同步改为等待实际页面/数据加载，并按真实目标商品 ID/全部库存核对零库存；先前失败不计作通过。

## 未执行与后续边界

父母实际试用、真实打印机纸面签收、正式云端部署、公网 DNS/正式 ACME 证书、真实数据迁入及异地实际复制均未执行。异地复制步骤已提供，未授权绑定外部账号或发送通知。完整验收的可选 Phase 3 历史调整单 ID 未提供；本阶段要求的历史空值快照/迁移保留由实际送货单 PDF 与双库升级契约覆盖。

隔离 cron 项在演练后已移除；隔离服务在证据核对后停止，业务卷及备份保留以便复查。开始人工试用时维护者重新启动隔离环境并按 `phase-5-trial.md` 记录反馈；根据反馈再修正相关任务。GitHub issue 状态不等同本地技术验收状态。
