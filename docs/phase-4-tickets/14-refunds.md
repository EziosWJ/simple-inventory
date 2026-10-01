# 14：待退款结算、客户退款、供应商退款与冲销

**GitHub issue:** [#28](https://github.com/EziosWJ/simple-inventory/issues/28) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者在退货或原交易取消形成待退款后，记录向客户退钱或收到供应商退款，分次结清并查询；录错通过冲销重录，保留实际资金和来源。

## Acceptance criteria

- [ ] 交付客户退款/供应商退款真实入口、确认表单、认证 API、记录列表/详情及冲销入口，复用既有往来余额/流水，不建逐单核销。
- [ ] 当前对应方向余额必须为负，正退款额不超过其绝对值；正/零余额退款、超额/负/零金额拒绝，支持分次退款。
- [ ] 保存即生效，记录方式、选填交易号/备注、业务日期、实际操作人/时间，前后余额明确，资金、余额、流水与审计原子提交。
- [ ] 退货先抵总欠款，超出才待退款；取消已收付款原交易形成的待退款也能处理，新的正确交易可先抵减待退款。
- [ ] 冲销必填原因，同原资金记录最多一次反向效果，原记录不编辑/删除；退款冲销恢复合法待退款，不误套普通收付款录入上限。
- [ ] 取消退货不删实际退款，重新欠款可继续普通收付；后续新交易/普通收付/退款/冲销竞争均基于事务内同方向最新余额。
- [ ] 同方向并发退款不超额，重复提交/冲销不重复变动，客户/供应商方向不抵扣，停用资料历史可结算。
- [ ] 双库覆盖部分退货未结款、结清后退货、已收付款交易取消、分次退款、取消退货新欠款、退款冲销、并发上限与审计回滚；实际页面演示。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #16：https://github.com/EziosWJ/simple-inventory/issues/16
- #25：https://github.com/EziosWJ/simple-inventory/issues/25
- #27：https://github.com/EziosWJ/simple-inventory/issues/27
