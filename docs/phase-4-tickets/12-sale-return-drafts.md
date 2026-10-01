# 12：销售退货草稿、原实物明细与查询

**GitHub issue:** [#26](https://github.com/EziosWJ/simple-inventory/issues/26) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者从原已过账销售单建立退货草稿，可选择多条原实物明细、填写部分数量，查看金额预览并编辑取消；不退服务、不立即改变库存或应收。

## Acceptance criteria

- [ ] 真实销售退货菜单、认证 API、原销售详情入口和草稿维护查询页面，单号/版本/状态与审计完整。
- [ ] 一张退货对应一张原 POSTED 销售单，原实物明细内部 ID 明确，服务和跨原单明细拒绝。
- [ ] 按原明细显示原数量、累计生效退货/剩余额度、原价/单位/描述；同商品有价/赠品分行的额度与价格独立。
- [ ] 只接受正三位数量，预览累计取整差额；零价/小数量零金额可保留，重复原明细不绕过额度。
- [ ] 停用客户/商品历史可退，无需重新启用；取消原单无新退货入口，草稿不能冒充最终额度已预占。
- [ ] 新建/编辑/取消不改变库存、往来金额或最终额度；旧版本拒绝保留输入，取消必填原因，原记录不删除。
- [ ] 分页/详情展示来源、创建/取消记录，多行不重计；双库验证角色停用、同商品不同价、服务拒绝、版本竞争和审计回滚，UI 可演示。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #21：https://github.com/EziosWJ/simple-inventory/issues/21
