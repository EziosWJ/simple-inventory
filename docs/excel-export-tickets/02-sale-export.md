# T02: 销售出库明细导出

Issue：[#87](https://github.com/EziosWJ/simple-inventory/issues/87)

**What to build:** 在销售出库页，按当前查询框的条件下载「销售明细」Excel 文件，复用 T01 建立的 xlsx 写入包与前端下载流程。

**Blocked by:** [#86](https://github.com/EziosWJ/simple-inventory/issues/86)

**Status:** 已实现并验收（2026-10-12）

- [x] 销售出库页两个视图下都能点「导出明细 Excel」，导出的都是明细
- [x] 导出行序与页面当前明细视图的排序一致（业务日期、成交单价、原金额、净销售额，含升降序）
- [x] 服务项目的退货列、草稿与已取消销售的净额列为空
- [x] 明细 sheet 共 18 列、首行为表头、末行为行数与原金额合计；合计 sheet 的金额三状态与数量分组与页面「数量合计」弹层口径一致
- [x] 数量与金额是数值单元格、日期与时点是文本
- [x] 文件名为 `销售明细_20260801-20261012.xlsx`；无日期条件时为 `销售明细_全部_20261012.xlsx`
- [x] 空结果仍产出含表头的合法文件；超过 10000 行先弹确认；超过 100000 行后端拒绝并给出中文提示
- [x] 双库契约测试各一份（`TestSQLite` 与 `TestPostgres` 前缀），通过 HTTP 取回 xlsx 后用 excelize 读回断言表头、单元格值、排序、留空规则与合计
- [x] `task backend:check`、`task api:docs`（核对生成差异）、`task frontend:lint`、`task frontend:build` 通过
- [x] 现有前端回归测试中按精确名称匹配「按单据」「按明细」按钮的用例仍然通过

## 集成注意

与 T01、T03 存在同文件冲突（前端业务接口文件、后端接口文档生成物），集成时按顺序合并。

实施与检查结果见 [交付记录](../excel-export-progress.md)。GitHub issue 的状态以线上记录为准。
