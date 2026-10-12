## Parent

#78

## What to build

直送一次确认同时生成并过账采购销售，交付可独立演示的端到端行为。

## Acceptance criteria

- [x] 客户收货后确认同事务生成并过账两单，库存净零、往来分别生效、快照与审计完整。
- [x] 失败回滚、重复请求、未知结果核实、零金额、服务和竞争均有双库契约。
- [x] 页面确认摘要、结果核实、生成单据跳转及复用送货打印可验收。

## Blocked by

- #79

## Local implementation

T02 已完成本地实现；SQLite/PostgreSQL 定向契约和真实浏览器主路径通过。完整验收状态见 [进度文档](../direct-delivery-redesign-progress.md)。
