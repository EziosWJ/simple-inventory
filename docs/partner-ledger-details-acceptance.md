# 往来明细弹窗与来源行内展开

2026-10-14：优化往来记录弹窗的金额层级与余额含义；“打开来源”改为“展示细节”，列表在本行下方展开，弹窗在当前内容中展开。来源查询沿用已有 API，不改变账务或库存规则。

## 验证范围

- `task frontend:lint`、`task frontend:build`、`git diff --check`。
- `node tests/ledger-source-details.mjs`：采购、销售、双向退货、期初、收付款、双向退款、交易冲销与资金冲销；列表行内与弹窗按需加载，URL 和页码保留，收起与失败重试，历史商品资料与服务，退货原单、来源取消状态、冲销追溯，缺失关联提示、迟到响应、翻页清除，长明细与 1440/1024/390px 弹窗固定关闭操作。全程无写请求。
- 已同步 `web/tests/phase5-partner-query.mjs` 的来源验收步骤为原地展开；本轮未运行该真实 HTTP 验收，其隔离 API 和造数链路不在本次专项内。

浏览器验收使用拦截 API；未改后端、REST 合同、数据库或事务，所以未运行后端检查与双库契约；未声称全业务浏览器回归通过。

日志：`/tmp/ledger-details-lint.log`、`/tmp/ledger-details-build.log`、`/tmp/ledger-source-details.log`。效果：`/tmp/ledger-source-row.png`、`/tmp/ledger-source-modal.png`，已查看截图。临时图片不作为仓库持久资产。
