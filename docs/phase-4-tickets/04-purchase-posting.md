# 04：采购整单过账、应付入账、取消与库存来源接入

**GitHub issue:** [#18](https://github.com/EziosWJ/simple-inventory/issues/18) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者确认已保存采购草稿，将整单实物入库并形成供应商应付，再从库存/往来记录打开来源采购单；录错时整单反向取消，历史和真实收付款保留。

## Acceptance criteria

- [ ] 开始库存接入前完成 #10 空值快照修复及 #13 旧来源详情验收；保留原调整路径、旧流水、余额及商品永久身份锁，不重复发布旧缺口修复。
- [ ] 过账携带核对版本，事务内校验最新 DRAFT、启用供应商/实物、类型/单位；按商品汇总数量，正向增加同一真实库存余额。
- [ ] 库存、原始流水、供应商方向往来金额、单据状态、实际过账人/时间与审计同事务提交，数量或金额超范围/任意写入失败整单回滚。
- [ ] 扩展现有库存流水的调整专属关联/原因以接受真实采购来源；保留旧逻辑，通过增量双库迁移验证旧数据，不把采购伪装成调整，不建平行库存余额。
- [ ] 过账冻结商品描述含空型号/规格，后续改名/补填不改变历史；类型/单位永久锁定，首次过账与商品编辑竞争一致。
- [ ] POSTED 锁定；取消必填原因、保留原库存/金额流水并追加反向记录，库存不足整单拒绝；原收付款保留，可形成待收供应商退款；支持后续退货任务注册生效退货阻塞。
- [ ] 重复及并发过账/取消仅一次效果，余额首次创建/多商品顺序一致；取消不解除商品锁，终态不能编辑或重新过账。
- [ ] 本任务即支持当前库存观察真实入库、库存流水识别采购/冲销并打开采购详情，往来余额增加/取消反向可追溯；最终多来源筛选由后续任务完善。
- [ ] 双库公开行为覆盖多行合计、并发首次入库、快照空值、商品锁竞争、已付款取消、库存不足取消、金额溢出和审计回滚；验证两种余额分别等于流水累计。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #16：https://github.com/EziosWJ/simple-inventory/issues/16
- #17：https://github.com/EziosWJ/simple-inventory/issues/17
- #10：https://github.com/EziosWJ/simple-inventory/issues/10
- #13：https://github.com/EziosWJ/simple-inventory/issues/13
