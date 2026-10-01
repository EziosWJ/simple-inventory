# 08：送货单 A4 打印、金额显隐与历史重印

**GitHub issue:** [#22](https://github.com/EziosWJ/simple-inventory/issues/22) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者从已保存销售草稿或已过账详情打印送货单，选择金额显隐，客户在纸面签收；商品或客户改名后重印旧单仍使用原资料。

## Acceptance criteria

- [ ] 交付真实打印入口及完整保存单据的只读打印数据路径，复用认证；草稿打印明确未过账，正式销售按冻结内容，已取消单不得冒充有效正式单。
- [ ] 包含经营者名称/联系资料、客户、本次送货地址/联系人、单号、业务日期、商品名称/型号/规格/单位/数量、备注及签收栏。
- [ ] 金额显示模式包含单价/行金额/合计，隐藏模式全部隐藏相关金额，不泄漏采购价格或内部成本。
- [ ] A4 纵向，长单自动分页、单号和页码、末页合计/签收栏，明细超过列表页大小仍完整打印；验证可重复打开和浏览器打印预览/PDF。
- [ ] 已过账重印使用客户/商品/经营者快照，包含历史空值；更新档案或经营者设置不改变旧单，草稿使用当前已保存内容。
- [ ] 打印不产生库存/往来变动，不把可变页面 DOM 或当前列表一页当唯一数据来源；无需电子签名/签收上传。
- [ ] 双库公开数据验证草稿/正式/取消状态、快照及权限；真实页面验收金额显隐与多页版式，lint/build 通过。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #21：https://github.com/EziosWJ/simple-inventory/issues/21
