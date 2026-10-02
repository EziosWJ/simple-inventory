# SQLite 暂时不可用时返回 HTTP 503

**GitHub issue:** [#37](https://github.com/EziosWJ/simple-inventory/issues/37) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

SQLite 数据库层在锁等待耗尽时已生成统一的暂时不可用错误，但 Phase 4 新增业务 handler 未统一映射该错误，部分写入接口返回 HTTP 500。项目 ADR-0006 要求该情形由 HTTP 层统一返回 503。

## Acceptance criteria

- [ ] 采购、销售、双向退货、往来结算/退款、经营者打印资料等新增业务 handler 将数据库层暂时不可用错误映射为 HTTP 503。
- [ ] 503 使用项目一致的错误响应封装；验证失败操作没有部分业务或审计写入。
- [ ] 参数错误、找不到资源、业务冲突仍分别维持既有 400、404、409 语义。
- [ ] 使用 handler contract 注入/触发标准暂时不可用错误，验证各业务入口返回 503；至少 SQLite 公共 HTTP contract 验证写锁等待耗尽路径。
- [ ] PostgreSQL 行为及正常 SQLite 操作不受影响。

## Related

- #14 Phase 4 规格
- [ADR-0006：原子操作与审计](../adr/0006-atomic-operation-audit.md)
- Phase 4 规格补充验收场景 16

## Validation

运行后端 tests/vet 和相关 SQLite、PostgreSQL 集成 contract；不需要 schema migration。
