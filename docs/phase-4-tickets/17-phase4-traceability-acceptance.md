# 17：库存多业务来源筛选、追溯贯通与整阶段验收

**GitHub issue:** [#31](https://github.com/EziosWJ/simple-inventory/issues/31) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者在当前库存进入商品流水，按采购、销售、退货、调整及原始/取消筛选，打开正确来源单据，再查看相应往来记录；完成本阶段全部真实用户路径和打印验收。

## Acceptance criteria

- [ ] 现有库存页面/认证分页 API 支持所有真实业务来源筛选，与 ORIGINAL/REVERSAL 正交；保留调整旧查询和 ID 兼容，不伪造统一调整来源。
- [ ] 流水展示当时商品快照含空值、增减数量、前后余额、实际执行人/时间、类型/单号及正确来源详情；现档案变更、停用、归零历史仍可查。
- [ ] 当前库存链接按内部商品 ID 预筛选，来源到整单全部明细/状态/创建过账取消记录，再到对应客户/供应商往来路径可追溯，不混用同商品分行与调整唯一商品规则。
- [ ] 验证库存余额等于全部实物原始与反向流水累计，客户/供应商各方向余额等于全部金额流水累计；服务无库存，负往来允许而负库存禁止。
- [ ] 完成采购→销售→分次收付款→部分退货→退款→逆序取消/资金冲销全路径，覆盖零价、同商品不同价、尾差、多行一项失败、并发及停用历史。
- [ ] 完成直送及直送退货、期初分次与冲销、已结清取消新待退款、已退款取消退货新欠款的真实 UI 演示，确认原记录均保留。
- [ ] 送货单金额显隐/未过账标记/快照重印和 A4 多页签收，对账多页/期初期末/跨期冲销真实预览或输出验证；超出列表页的记录完整。
- [ ] PostgreSQL/SQLite 空库及 Phase 3 增量升级、Swagger 重新生成、任务/CI 确实选中新 contract；task check 与 task db:check 通过，明确失败/跳过/未执行项。
- [ ] 本任务不重复重建前序模块，只补齐多来源筛选和追溯用户路径及真实回归，实际缺口在相关公开行为中修复。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #22：https://github.com/EziosWJ/simple-inventory/issues/22
- #29：https://github.com/EziosWJ/simple-inventory/issues/29
- #30：https://github.com/EziosWJ/simple-inventory/issues/30
