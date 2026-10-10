# 零价销售与北京时间显示

2026-10-10 用户确认：销售成交单价允许 0；所有前端时间展示、默认当天日期和整日筛选统一按北京时间（Asia/Shanghai，UTC+8），业务日期原样保留，API 继续使用 UTC。

## 零价销售排查

反馈发生于 `jxc.ezios.cn`：2026-10-09 录入鼠标销售时，金额填写 0 无法提交，改为 1。尚未取得当时的按钮状态或错误提示。

规格、前端金额校验及后端 Service 原本均支持零单价和整单零金额，本轮没有修改金额业务规则。

- 隔离 SQLite API 上运行 `PHASE5_API_URL=http://127.0.0.1:18119 node tests/phase5-sale-post.mjs` 通过：真实保存/过账包含同商品两行零价实物、零价服务、库存不足、直送及请求恢复。
- `node tests/zero-sale.mjs` 通过：真实表单选鼠标，单价从 20 清空时保存禁用，填写 0 后可以保存并发起过账。API 全部拦截；洛杉矶浏览器在北京时间 00:15 默认正确业务日期。
- 只读下载线上公开静态资源，在完全拦截 API 的隔离环境运行 `ZERO_SALE_ASSET_DIR=/tmp/si-live-zero-assets node tests/zero-sale.mjs` 通过。线上 JS 为 `/assets/index-f5LRtJ_x.js`，SHA256 为 `2ebce882f4b368a887028f6ddf631618f119a78462ebc17f63a6b382781380a2`，与 10 月 8 日发布记录一致。
- 线上公开前端可以发出零价保存及过账请求；这不代表已验证生产后端或重现了昨天的问题。没有写线上业务数据、没有改动那张金额为 1 的销售单。

## 时间修复

原公共格式化函数仅截取 UTC 字符串，未转换时区；业务页面另使用浏览器本地时区，两者不一致。`node tests/datetime.mjs` 修复前显示 `2026-10-09 16:15:00`，期望北京时间 `2026-10-10 00:15:00`，断言失败。

统一 `web/src/lib/datetime.ts`，覆盖系统日志/通知、业务详情及附件/直送历史、对账打印、采购/销售/退货默认日期、库存及往来期间日界和期间回填。无时区的旧时间文本按 UTC 解释；纯业务日期不转换。时间展示统一为 `YYYY-MM-DD HH:mm:ss`。

## 检查与边界

通过：`npm run test:datetime`（UTC、洛杉矶、上海三种环境时区）、零价 UI 回归、本地真实销售过账回归、HTTP 保存回归、`task frontend:lint`、`task frontend:build`、`git diff --check`。

`PRINT_LAYOUT_KIND=statement node tests/print-layout.mjs` 通过：洛杉矶浏览器仍显示北京时间，生成真实 PDF 并验证 A4 边界、页码、末页合计。客户多页 8 页、供应商长文本 9 页、空账单 1 页。证据目录 `/tmp/simple-inventory-print-layout-pdfs`。

完整 `node tests/print-layout.mjs` 在首项送货单 `delivery-visible` 的单号宽度断言失败；该场景不涉及本次修改的对账时间，未将全量打印验收记为通过。

本次没有修改后端、API 或数据库结构，未运行全量 `task backend:check`、`task db:check`；真实零价过账验证使用隔离 SQLite，未定向验证 PostgreSQL。保留已有工作区改动。时间修改尚未部署。
