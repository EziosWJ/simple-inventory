# 15：直送退货两边关联追溯与操作路径

**GitHub issue:** [#29](https://github.com/EziosWJ/simple-inventory/issues/29) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

客户直接退货给供应商时，经营者从直送关联单先建立销售退货，再建立采购退货，分别确认并查看两边数量、金额和状态。

## Acceptance criteria

- [ ] 直送采购/销售详情提供对应原单及退货入口，退货各自关联单个原单和明细，不把两边价格/金额合并。
- [ ] 明确先销售退货入库、后采购退货出库，分别过账且中间库存可暂增；支持分次部分退货，沿用每边原明细额度和原价。
- [ ] 两边原采购/销售及对应退货可以交叉追溯状态/实际操作记录，取消关联历史保留；不增加两张退货单的一键联合生效或自动退款。
- [ ] 任一侧失败不影响另一侧已完成事实，清楚提示当前状态与后续操作；库存不足/额度变化整侧拒绝。
- [ ] 取消/逆序额度规则复用普通退货，原单有生效退货仍不能取消；客户应收与供应商应付分别表达。
- [ ] 双库公开路径及浏览器验收直送→销售部分退货→采购部分退货→来源追溯/取消竞争，结果与各自数量/金额流水一致。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #23：https://github.com/EziosWJ/simple-inventory/issues/23
- #25：https://github.com/EziosWJ/simple-inventory/issues/25
- #27：https://github.com/EziosWJ/simple-inventory/issues/27
