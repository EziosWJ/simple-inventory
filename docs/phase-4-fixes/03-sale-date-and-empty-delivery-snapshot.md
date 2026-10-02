# 修正销售默认业务日期并保留明确清空的送货资料

**GitHub issue:** [#34](https://github.com/EziosWJ/simple-inventory/issues/34) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

销售草稿用 UTC 日期初始化业务日期；上海当地午夜后但 UTC 尚未跨日时会默认到前一天。另一个问题是服务层把明确传入的空送货联系人、电话、地址归一成“未提供”，新建时仓储层遂用往来单位档案资料回填，用户无法把本次送货字段留空。

## Acceptance criteria

- [ ] 新建销售草稿的默认业务日期使用用户本地日历日，不受 UTC 跨日差异影响。
- [ ] 业务日期仍作为日期值传递和保存；不会因浏览器时区转换前后漂移。
- [ ] 新单字段未提供时，送货资料继续默认使用往来单位档案值；用户明确清空字段时，空值与未提供可区分且保存为空。
- [ ] 明确清空的联系人、电话、地址在草稿读取、过账快照和已保存销售送货单重印中保持为空，不被后续档案值覆盖。
- [ ] 验证 Asia/Shanghai 当地 00:15 新建单据默认当天日期；分别验证字段省略、填写及明确清空三种输入。

## Related

- #19 销售草稿
- #21 销售过账与快照
- Phase 4 规格补充验收场景 13

## Validation

运行前端 lint/build，并以 SQLite、PostgreSQL 公开 HTTP contract 验证输入区分、持久快照和过账后读取。时间 UI 场景使用 Asia/Shanghai 时区。
