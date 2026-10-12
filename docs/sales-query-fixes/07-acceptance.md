# 销售查询整改整体验收与效果交付

## Parent

https://github.com/EziosWJ/simple-inventory/issues/66

## What to build

完成所有整改后的完整销售查询路径，形成可供经营者实际查看的页面效果和可复跑的验收记录。

## Acceptance criteria

- [x] 逐一核对前六项验收条件，并覆盖客户期间查询和商品历史成交价查询的完整路径；T06 新布局已复验。
- [x] 组合验证输入框内置浏览、快速选择后立即输入日期、切换视图、排序、翻页、打开原单、返回和刷新，筛选及页码语义一致。
- [x] 核对跨期有效退货、退货取消、不同状态、服务、零金额和全结果合计；T06 新布局调整未改变计算口径。
- [x] 验证已提交与未提交条件、详情请求隔离、错误/加载状态及迟到响应；刷新后越界页被纠正。
- [x] 使用双库 HTTP 契约及真实浏览器验收，执行最终前端 lint/build、数据库兼容门禁；T06 新布局完成后复跑相关浏览器验收，REST API 改动核对文档生成差异。
- [x] 共用组件或钩子发生变化时定向回归相关调用方，其他业务页面的展示不被主动推广。
- [x] 提供反映新布局的代表性桌面/窄屏截图、检查记录、复跑入口与实际未执行项；经营者试用结果单独记录，自动检查不能冒充人工反馈。
- [x] 记录父规格引用与整改交付范围；不自动关闭或修改父 Issue，不将交付验收视为正式服务器部署授权。

## Blocked by

- [销售查询状态隔离与请求依赖整理 (#67)](https://github.com/EziosWJ/simple-inventory/issues/67)
- [销售查询错误反馈、合计加载状态与页码校正 (#72)](https://github.com/EziosWJ/simple-inventory/issues/72)
- [销售筛选输入框内置浏览选择入口 (#69)](https://github.com/EziosWJ/simple-inventory/issues/69)
- [销售商品查询匹配明细与高亮统一 (#71)](https://github.com/EziosWJ/simple-inventory/issues/71)
- [销售过账确认准确表达商品与库存影响 (#70)](https://github.com/EziosWJ/simple-inventory/issues/70)
- [销售明细表与全结果合计可读性调整 (#68)](https://github.com/EziosWJ/simple-inventory/issues/68)


## 本地交付

状态：本地整体验收完成（2026-10-12）。T06 页面反馈已实现并通过桌面/窄屏复验。经营者人工试用仍单独记录在 [交付记录](acceptance.md)。
