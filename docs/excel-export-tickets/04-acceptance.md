# T04: 三页导出一致性验收与回归

Issue：[#89](https://github.com/EziosWJ/simple-inventory/issues/89)

**What to build:** 三个页面的导出能力都落地后，做一次跨页一致性核对与回归，确认三份文件是同一套规则生成的，并对既有前端交互做回归。跨页一致性只有三页都存在时才可能核对，因此本任务单独成立。

**Blocked by:** [#86](https://github.com/EziosWJ/simple-inventory/issues/86)、[#87](https://github.com/EziosWJ/simple-inventory/issues/87)、[#88](https://github.com/EziosWJ/simple-inventory/issues/88)

**Status:** 已实现并验收（2026-10-12）

- [x] 三页的 sheet 命名、列命名、文件名规则、单元格类型规则、确认阈值与上限文案完全一致，差异只允许出现在列集合与合计区块的业务口径上
- [x] 三页的导出都由同一份 xlsx 写入包与前端下载流程驱动，没有重复实现
- [x] 接口文档生成差异已核对，非导出、非直送单改造的改动为零
- [x] 既有前端业务回归测试通过，尤其是按精确名称匹配视图切换按钮的用例
- [x] 导出前后采购、销售、库存余额与往来余额数据一致
- [x] 三个页面的导出在 PostgreSQL 与 SQLite 上各验证一次
- [x] 收尾检查通过：`task backend:check`、`task api:docs`、受影响的定向双库契约、`task frontend:lint`、`task frontend:build`

## 边界

本任务是核对与回归，不引入新功能；发现的不一致按原因回退到 T01、T02 或 T03 修复。

实施与检查结果见 [交付记录](../excel-export-progress.md)。GitHub issue 的状态以线上记录为准。
