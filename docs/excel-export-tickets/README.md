# 导出 Excel 任务

父规格：[GitHub #85](https://github.com/EziosWJ/simple-inventory/issues/85)，本地见 [导出规格](../excel-export-spec.md)。

任务已按用户确认的拆分发布为 GitHub issue，原生阻塞关系已核对。三个页面共享后端 xlsx 写入包与前端下载流程，因此 T01 是打通全链路的 tracer bullet，T02 与 T03 只依赖它、彼此独立，T04 依赖全部。

| 任务 | Issue | 阻塞 | 状态 |
| --- | --- | --- | --- |
| T01 采购入库明细导出（含三页共用导出工具） | [#86](https://github.com/EziosWJ/simple-inventory/issues/86) | 无 | 已实现并验收 |
| T02 销售出库明细导出 | [#87](https://github.com/EziosWJ/simple-inventory/issues/87) | #86 | 已实现并验收 |
| T03 往来明细导出 | [#88](https://github.com/EziosWJ/simple-inventory/issues/88) | #86 | 已实现并验收 |
| T04 三页导出一致性验收与回归 | [#89](https://github.com/EziosWJ/simple-inventory/issues/89) | #86, #87, #88 | 已实现并验收 |

实现与定向双库、浏览器验收记录见 [交付记录](../excel-export-progress.md)。

## 并行与集成

T02 与 T03 相互独立，但都会改动前端业务接口文件与后端接口文档生成物。并行时按 CLAUDE.md 的节奏使用独立工作树，先约定文件归属，由一人顺序集成；不能并行合并同一份生成物。

## 本地任务文件

- [T01 采购入库明细导出](01-purchase-export.md)
- [T02 销售出库明细导出](02-sale-export.md)
- [T03 往来明细导出](03-partner-ledger-export.md)
- [T04 三页导出一致性验收与回归](04-acceptance.md)

本地实现状态不表示代码已推送或部署；GitHub issue 保持打开。
