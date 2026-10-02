# 退货草稿编辑按稳定原明细 ID 对应

**GitHub issue:** [#32](https://github.com/EziosWJ/simple-inventory/issues/32) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

采购和销售退货编辑时，表单把退货草稿的明细子集按数组下标映射到原单的完整明细。若原单有 A、B 两行而草稿只退 B，B 的数量会显示在 A 下；用户输入的数量仍绑定到 B，其他输入也可能被丢弃，容易造成误退。

## Acceptance criteria

- [ ] 采购、销售退货编辑按稳定的原采购/销售明细 ID 对应表单行，不依赖数组位置。
- [ ] 原单包含多行、退货草稿只含非首行时，正确的商品显示已有数量；修改、添加、清空数量后只提交用户对应的原明细 ID 和值。
- [ ] 已有明细不能重复提交或串到另一条原明细；新增/移除草稿行仍可正常工作。
- [ ] 采购与销售各有回归验证：只退第二条原明细 B，编辑时修改 B 不改变 A，保存的请求仍关联 B；读取保存结果确认未丢失或错配输入。
- [ ] 不改变后端原单额度、分行定价、累计尾差及过账规则。

## Related

- #24 采购退货草稿
- #26 销售退货草稿
- Phase 4 规格补充验收场景 11

## Validation

验证相关前端 lint/build，并用真实 UI 操作或组件行为测试覆盖采购和销售两条编辑路径；若改动 API/后端则补相应双库公开 HTTP contract。
