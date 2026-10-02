# 执行直送退货先销售后采购的顺序规则

**GitHub issue:** [#35](https://github.com/EziosWJ/simple-inventory/issues/35) · `ready-for-agent`

## Parent

Phase 4 规格：[GitHub #14](https://github.com/EziosWJ/simple-inventory/issues/14) · [本地规格](../phase-4-spec.md)

## Problem

直送采购和销售分别过账后，当前采购退货只校验共享库存与采购原单额度。若库存还有其他来源数量，即使关联销售没有退货，也能先把直送采购退回供应商，绕过已确认的“先销售退货后采购退货”流程。

## Acceptance criteria

- [ ] 直送采购退货过账前检查关联销售已有已过账退货；尚无关联销售退货时拒绝采购退货，即使当前库存足够。
- [ ] 被拒绝时退货状态、库存流水/余额、供应商往来余额和额度均不变，草稿仍可继续处理。
- [ ] 关联销售退货先成功过账后，采购退货可继续处理，并仍独立满足采购原明细累计数量、金额尾差和当前库存校验。
- [ ] 普通非直送采购退货不受该顺序规则影响；不联合过账两张退货单，也不新增长度更强的自动数量分配规则。
- [ ] SQLite 与 PostgreSQL contract 覆盖额外库存可绕过的场景、拒绝后原数据不变、销售退货先过账后可继续采购退货，以及并发/重复请求不重复生效。
- [ ] UI 明确提示先办理关联销售退货，并能沿关联来源进入对应销售退货流程。

## Related

- #23 直送关联
- #29 直送退货追溯
- Phase 4 规格补充验收场景 14

## Validation

运行相关双库公开 HTTP contract、后端 tests/vet 和前端 lint/build。
