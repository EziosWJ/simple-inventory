# Phase 4 真实浏览器验收

脚本使用现有 API/Vite、真实 JWT 登录和 UI 操作，不启动服务、不使用 mock。
**仅对隔离测试数据库运行**：它会创建商品、往来单位、交易和资金记录，并更新经营者打印资料及自己创建的档案。

从仓库根目录执行 `node web/tests/phase4-acceptance.mjs`，先通过环境变量提供：

- `ACCEPTANCE_API_URL`：正在运行的 API 地址。
- `ACCEPTANCE_WEB_URL`：对应 API 的前端地址。
- `ACCEPTANCE_USERNAME` / `ACCEPTANCE_PASSWORD`：测试账号；脚本不记录密码或 token。
- `PLAYWRIGHT_EXECUTABLE_PATH`：可选 Chromium 路径，未指定则使用 Playwright 已安装浏览器。
- `ACCEPTANCE_OUTPUT_DIR`：可选证据目录，默认 `.task/phase4-acceptance`。
- `ACCEPTANCE_LEGACY_ADJUSTMENT_ID`：可选 Phase 3 升级前已过账调整单 ID，含空型号/规格历史快照。
- `ACCEPTANCE_QUOTA_CHECK`：可选 quota 检查 Python 脚本路径，低于 15% 或请求停止时保存证据退出。

需要 Node、web 已安装依赖、Python 3，以及 Poppler 的 `pdfinfo`、`pdftotext`、`pdftoppm`。
`RESUME=1` 可读取输出目录的 `browser-result.json`，跳过已通过阶段；仅在原测试数据库与证据完整保留时使用。

覆盖普通采购/销售（同商品分价、零价、服务）、分次收付款、部分退货、退款、退货与原单取消、资金冲销、直送分别过账及退货、期初分次及合法冲销、库存来源详情/往来跳转和余额累计。打印使用真实已保存单据：40 行送货三页、金额显隐、草稿水印、过账快照重印、43 条完整期间往来四页含冲销。

输出 JSON、截图、PDF 和末页栅格预览；PDF 检查真实物理页数、A4 尺寸、每页页码、文字边界和末页合计/签收，避免只验证 DOM 中的预览页数。证据不提交到 Git。跨库 HTTP 合同和并发边界由后端数据库门禁负责。
