## Parent

#78

## What to build

普通销售多采购明细来源与售后追溯，交付可独立演示的端到端行为。

## Acceptance criteria

- [x] 按销售明细选多个同商品已过账采购明细并记录数量，选填和部分记录、合计上限、日期确认由 API 和界面共同保证。
- [x] 过账后可补填更正，有版本校验与前后审计；不占用采购数量、不影响库存或往来。
- [x] 来源后续取消退货不丢失，显示状态及退货情况；修改商品数量不会留下无效来源。
- [x] 普通新销售取消原直送选择器，旧草稿保留原入口；双库及浏览器完整路径通过。

## Blocked by

None (can start immediately).

## Local implementation

T04 已完成本地实现；双库定向契约和真实浏览器场景通过。完整验收状态见 [进度文档](../direct-delivery-redesign-progress.md)。
