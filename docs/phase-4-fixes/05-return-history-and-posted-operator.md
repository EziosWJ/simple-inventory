# 保留退货历史查询并展示销售退货过账人

**GitHub issue:** [#36](https://github.com/EziosWJ/simple-inventory/issues/36) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

销售退货详情和分页查询没有填充过账用户，界面无法显示真实过账人及过账时间。另在退货原明细允许的数量范围内，已取消退货记录详情仍把自己的历史数量与当前有效退货量求和；当两者之和溢出内部整数范围时，已取消的旧单也无法读取。

## Acceptance criteria

- [ ] 销售退货列表和详情返回实际过账用户；已过账记录在 UI 显示过账人及时间，创建人不替代过账人。
- [ ] 采购、销售的已取消退货仍能通过详情 API 和分页列表读取完整保存内容；数量/金额运算只使用当前生效退货计算额度，历史取消数量不得重新加入累计值。
- [ ] 构造当前数量精度与存储范围内合法、取消后重新退货会使“当前数量 + 被取消数量”溢出的场景；旧取消单及分页结果仍为成功，结果不改写历史值。
- [ ] 取消单不占用可退额度，重新退货的累计取整仍按当前生效退货准确计算。
- [ ] SQLite 与 PostgreSQL 分别验证详情/分页响应、过账人和额度累计；原单、库存和往来业务规则不变。

## Related

- #25 采购退货过账
- #27 销售退货过账
- Phase 4 规格补充验收场景 15

## Validation

运行相关双库公开 HTTP contract、后端 tests/vet 和前端 lint/build。
