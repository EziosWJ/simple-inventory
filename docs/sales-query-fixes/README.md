# 销售出库整改任务

状态：T01～T07 已完成本地实现与验收；T06 (#68) 已按页面反馈调整合计 panel 和数量查看方式，并通过桌面/窄屏复验。检查与效果见 [交付记录](acceptance.md)。GitHub #67～#73 已关闭，父规格 #66 未修改。

来源：销售查询规格 #66 交付后的截图与代码检查，以及输入框内置浏览选择的补充需求。采用可输入搜索加尾部浏览图标，先在销售出库启用。

| 编号 | 任务 | 真正阻塞 | 任务正文 |
| --- | --- | --- | --- |
| T01 | [销售查询状态隔离与请求依赖整理 (#67)](https://github.com/EziosWJ/simple-inventory/issues/67) | 无 | [正文](01-query-state.md) |
| T02 | [销售查询错误反馈、合计加载状态与页码校正 (#72)](https://github.com/EziosWJ/simple-inventory/issues/72) | #67 | [正文](02-query-feedback.md) |
| T03 | [销售筛选输入框内置浏览选择入口 (#69)](https://github.com/EziosWJ/simple-inventory/issues/69) | 无 | [正文](03-inline-picker.md) |
| T04 | [销售商品查询匹配明细与高亮统一 (#71)](https://github.com/EziosWJ/simple-inventory/issues/71) | 无 | [正文](04-match-indicators.md) |
| T05 | [销售过账确认准确表达商品与库存影响 (#70)](https://github.com/EziosWJ/simple-inventory/issues/70) | 无 | [正文](05-posting-impact.md) |
| T06 | [销售明细表与全结果合计可读性调整 (#68)](https://github.com/EziosWJ/simple-inventory/issues/68) | 无 | [正文](06-readable-results.md) |
| T07 | [销售查询整改整体验收与效果交付 (#73)](https://github.com/EziosWJ/simple-inventory/issues/73) | #67～#72 | [正文](07-acceptance.md) |

T01 将必要的局部整理并入可验收的查询状态修复；T02 依赖其状态与请求入口。其余功能可独立实现，T07 等全部整改完成后验收。共用文件的修改冲突通过顺序集成协调，不把文件重叠当作业务阻塞。

涉及前后端的任务贯通 API、展示及验收；纯前端任务无需人为新增 schema 或后端修改。票据正文不固定文件路径。已按依赖顺序发布、标记 ready-for-agent，并使用 GitHub 原生阻塞关系；未修改或关闭 #66。

最初发布批次仅建立任务及索引；本轮已落实业务代码、契约、浏览器验收和效果交付。复跑入口：`bash web/tests/sales-query-fixes.sh`。
