# Phase 4 实现任务

规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) / [本地规格](../phase-4-spec.md)。用户已授权直接发布；17 个任务均已发布，标记 `ready-for-agent`，使用 GitHub 原生子任务及 blocked-by 关系。每项交付真实业务、所需迁移、认证 API、菜单/UI、审计、Swagger 与双库验证。

| GitHub issue | 任务 | Blocked by |
| --- | --- | --- |
| [#15](https://github.com/EziosWJ/simple-inventory/issues/15) | [期初应收应付录入、往来余额与冲销](01-opening-balances.md) | 无 |
| [#16](https://github.com/EziosWJ/simple-inventory/issues/16) | [按往来总欠款记录收付款、分次结清与冲销](02-settlements.md) | [#15](https://github.com/EziosWJ/simple-inventory/issues/15) |
| [#17](https://github.com/EziosWJ/simple-inventory/issues/17) | [采购入库草稿建档、编辑、取消与查询](03-purchase-drafts.md) | 无 |
| [#18](https://github.com/EziosWJ/simple-inventory/issues/18) | [采购整单过账、应付入账、取消与库存来源接入](04-purchase-posting.md) | [#16](https://github.com/EziosWJ/simple-inventory/issues/16)、[#17](https://github.com/EziosWJ/simple-inventory/issues/17)、[#10](https://github.com/EziosWJ/simple-inventory/issues/10)、[#13](https://github.com/EziosWJ/simple-inventory/issues/13) |
| [#19](https://github.com/EziosWJ/simple-inventory/issues/19) | [销售出库草稿、实物服务混合与查询](05-sale-drafts.md) | [#18](https://github.com/EziosWJ/simple-inventory/issues/18) |
| [#20](https://github.com/EziosWJ/simple-inventory/issues/20) | [经营者打印资料统一维护与审计](06-print-profile.md) | 无 |
| [#21](https://github.com/EziosWJ/simple-inventory/issues/21) | [销售整单出库、应收入账、历史快照与取消](07-sale-posting.md) | [#19](https://github.com/EziosWJ/simple-inventory/issues/19)、[#20](https://github.com/EziosWJ/simple-inventory/issues/20) |
| [#22](https://github.com/EziosWJ/simple-inventory/issues/22) | [送货单 A4 打印、金额显隐与历史重印](08-delivery-print.md) | [#21](https://github.com/EziosWJ/simple-inventory/issues/21) |
| [#23](https://github.com/EziosWJ/simple-inventory/issues/23) | [直送采购销售关联、数量核对与取消顺序](09-direct-delivery.md) | [#21](https://github.com/EziosWJ/simple-inventory/issues/21) |
| [#24](https://github.com/EziosWJ/simple-inventory/issues/24) | [采购退货草稿、原明细选择与查询](10-purchase-return-drafts.md) | [#18](https://github.com/EziosWJ/simple-inventory/issues/18) |
| [#25](https://github.com/EziosWJ/simple-inventory/issues/25) | [采购退货过账、金额尾差与逆序取消](11-purchase-return-posting.md) | [#24](https://github.com/EziosWJ/simple-inventory/issues/24) |
| [#26](https://github.com/EziosWJ/simple-inventory/issues/26) | [销售退货草稿、原实物明细与查询](12-sale-return-drafts.md) | [#21](https://github.com/EziosWJ/simple-inventory/issues/21) |
| [#27](https://github.com/EziosWJ/simple-inventory/issues/27) | [销售退货入库、应收抵减与逆序取消](13-sale-return-posting.md) | [#26](https://github.com/EziosWJ/simple-inventory/issues/26) |
| [#28](https://github.com/EziosWJ/simple-inventory/issues/28) | [待退款结算、客户退款、供应商退款与冲销](14-refunds.md) | [#16](https://github.com/EziosWJ/simple-inventory/issues/16)、[#25](https://github.com/EziosWJ/simple-inventory/issues/25)、[#27](https://github.com/EziosWJ/simple-inventory/issues/27) |
| [#29](https://github.com/EziosWJ/simple-inventory/issues/29) | [直送退货两边关联追溯与操作路径](15-direct-returns.md) | [#23](https://github.com/EziosWJ/simple-inventory/issues/23)、[#25](https://github.com/EziosWJ/simple-inventory/issues/25)、[#27](https://github.com/EziosWJ/simple-inventory/issues/27) |
| [#30](https://github.com/EziosWJ/simple-inventory/issues/30) | [往来明细分页、实际生效期间对账与 A4 打印](16-partner-statements.md) | [#28](https://github.com/EziosWJ/simple-inventory/issues/28) |
| [#31](https://github.com/EziosWJ/simple-inventory/issues/31) | [库存多业务来源筛选、追溯贯通与整阶段验收](17-phase4-traceability-acceptance.md) | [#22](https://github.com/EziosWJ/simple-inventory/issues/22)、[#29](https://github.com/EziosWJ/simple-inventory/issues/29)、[#30](https://github.com/EziosWJ/simple-inventory/issues/30) |

## 推进规则

- 当前可独立开始：#15 期初应收应付、#17 采购草稿、#20 经营者打印资料。完成 #15 后可做 #16 收付款。
- 采购→销售交付顺序已确认，销售草稿 #19 在采购过账 #18 交付后推进；期初往来及经营者资料可先行。
- #18 首次接入库存时须先完成外部 #10、#13 的快照修复与验收，不重复创建旧缺口修复 issue。
- 按阻塞关系选择前置任务都已完成的任务，正文和原生关系使用相同 issue 编号。
- 发布不代表实现完成；Phase 3 #7、#10、#13 的现有状态不在本次发布中变更。
