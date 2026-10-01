# 10：采购退货草稿、原明细选择与查询

**GitHub issue:** [#24](https://github.com/EziosWJ/simple-inventory/issues/24) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者从已过账采购单建立一张退货草稿，选择该单实物明细并填正退货数量，查询或编辑取消；草稿不占用最终额度、不减少库存。

## Acceptance criteria

- [ ] 真实采购退货入口、认证草稿 API、原采购单入口、分页/详情/编辑/取消页面交付。
- [ ] 一张退货只对应一张原 POSTED 采购单，可多行原明细、不跨原单；稳定原明细 ID 识别同商品不同价，不用当前商品选择代替来源。
- [ ] 展示原数量、累计仍生效退货、剩余额度、原成交价/单位/快照；只接受正三位数量，草稿额度展示是参考，最终过账重校验。
- [ ] 停用原商品/供应商无需启用即可建历史退货；取消原单或服务明细不可选，重复选择原明细不能绕过额度。
- [ ] 金额预览按累计数量原价取整减已有退货金额，允许零金额；保存不改变库存、往来余额和最终额度。
- [ ] 草稿唯一单号/版本、冲突保护、取消原因与历史，查询按原单/供应商/状态/日期稳定分页，审计与保存原子。
- [ ] 双库公开行为覆盖单原单、多价格原明细、停用、无效来源/数量、版本与审计回滚；页面演示原采购→退货草稿。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #18：https://github.com/EziosWJ/simple-inventory/issues/18
