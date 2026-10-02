# 采购与销售草稿编辑时重新确认商品单位

**GitHub issue:** [#33](https://github.com/EziosWJ/simple-inventory/issues/33) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

采购、销售草稿编辑表单从当前商品档案取单位并直接保存。商品单位从“小时”改为“次”后，旧草稿只需打开并保存，就会把原数量按新单位表达，没有用户确认，可能改变实际业务含义。

## Acceptance criteria

- [ ] 采购和销售草稿读取、编辑时保留单据当前保存的商品类型和单位。
- [ ] 发现档案类型或单位与草稿不一致时，明确提示变化；须由用户确认后才能按新类型/单位保存，不能静默替换或换算数量。
- [ ] 不确认时保留原草稿字段和值；过账仍按后端现有规则重新校验商品及单据一致性。
- [ ] 分别验证采购实物与销售实物/服务：把商品单位从“小时”改为“次”，打开并直接保存旧草稿，原单位和数量不变；确认更改后记录的是确认后的单位和值。
- [ ] 不引入多单位换算。

## Related

- #17 采购草稿
- #19 销售草稿
- Phase 4 规格补充验收场景 12

## Validation

运行前端 lint/build；若实现涉及后端规则，增加采购、销售公开 HTTP contract，并分别验证 SQLite、PostgreSQL。
