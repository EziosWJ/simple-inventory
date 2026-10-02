# 05：销售保存并过账与异常恢复

**GitHub issue:** [#44](https://github.com/EziosWJ/simple-inventory/issues/44) · `ready-for-agent`

## Parent

[Phase 5 规格 #39](https://github.com/EziosWJ/simple-inventory/issues/39)

## What to build

经营者直接确认销售并保存过账；库存不足留下原草稿，纯服务与零金额正常生效，网络中断和双人竞争不会重复出库或记应收。

## Acceptance criteria

- [ ] 复用采购交付的保存并过账交互，显示销售对象、明细、合计、实物扣减及应收影响；取消确认不提交。
- [ ] 先可靠保存后以返回的销售 ID/版本过账；保存失败不出库，过账失败保留同一草稿和具体原因。
- [ ] 真实结果核实覆盖保存/过账丢响应、重复提交、版本变化、他人取消及核实尚未完成；不自动新建或重放整张销售。
- [ ] 同商品分行库存按商品合计，服务不产生库存；零金额仍完成状态/库存/审计，不制造零金额往来流水。
- [ ] 直送关联、汇总数量一致及先采购后销售约束继续执行，不增加两单联合过账。
- [ ] 已过账快照、明确空送货资料、实际操作人及有效时间保留；库存、应收、状态及审计一次原子生效。
- [ ] 文件型 SQLite 和 PostgreSQL 公开契约覆盖库存不足整单回滚、纯服务、零价、分行、直送及网络/并发边界，浏览器实际操作确认。
- [ ] 送货打印入口和来源导航回归；相关门禁和 Swagger 同步完成。

## Blocked by

- [T03：采购保存并过账与操作结果核实（#42）](https://github.com/EziosWJ/simple-inventory/issues/42)
- [T04：销售完整录单页、搜索与可靠草稿保存（#43）](https://github.com/EziosWJ/simple-inventory/issues/43)
