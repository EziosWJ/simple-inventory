# Phase 3 实现任务

规格：[GitHub #7](https://github.com/EziosWJ/simple-inventory/issues/7) / [本地规格](../phase-3-spec.md)。全部任务已发布并标记 `ready-for-agent`，使用 GitHub 原生子任务与阻塞依赖；本地每个文件保留对应正文。

| GitHub issue | 任务 | Blocked by |
| --- | --- | --- |
| [#8](https://github.com/EziosWJ/simple-inventory/issues/8) | [库存调整单草稿建档、详情和分页查询](01-adjustment-create-query.md) | 无，当前工作区已实现 |
| [#9](https://github.com/EziosWJ/simple-inventory/issues/9) | [库存调整草稿编辑、版本冲突与草稿取消](02-adjustment-edit-draft-cancel.md) | #8；当前工作区已实现 |
| [#10](https://github.com/EziosWJ/simple-inventory/issues/10) | [库存调整整单过账、非负余额与商品引用锁定](03-adjustment-post.md) | #9；当前工作区已实现 |
| [#11](https://github.com/EziosWJ/simple-inventory/issues/11) | [已过账库存调整取消、反向流水与纠错路径](04-adjustment-posted-cancel.md) | #10；当前工作区已实现 |
| [#12](https://github.com/EziosWJ/simple-inventory/issues/12) | [当前库存分页查询、零库存视图与停用商品展示](05-inventory-balances.md) | #10；当前工作区已实现 |
| [#13](https://github.com/EziosWJ/simple-inventory/issues/13) | [库存流水分页查询、原始与冲销筛选及来源追溯](06-inventory-entries-traceability.md) | #11、#12；当前工作区已实现 |

按阻塞关系推进；#8～#13 已在当前工作区全部实现，Phase 3 库存基础范围（草稿、过账、冲销、当前库存、流水追溯）已完成，未实现项为采购/销售/报表/打印。GitHub issue 的关闭与依赖解除待代码提交和合并后处理。
