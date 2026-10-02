# 业务操作结果反馈

对应任务：[#54 采购销售反馈](https://github.com/EziosWJ/simple-inventory/issues/54)、[#55 库存调整与退货反馈](https://github.com/EziosWJ/simple-inventory/issues/55)、[#56 往来结算反馈](https://github.com/EziosWJ/simple-inventory/issues/56)。#55、#56 均依赖 #54 提供的公共反馈能力。

## 交互约定

业务操作由页面的操作处理函数发布反馈，复用全局 Toast，并在当前页面保留带图标、标题、单号、状态和业务影响的结果卡片。普通读取、刷新和重渲染不发布成功气泡。保存草稿约 3 秒；过账、取消、冲销、资金生效约 6 秒；错误和警告约 8 秒，均可手动关闭。气泡在弹窗上方显示，关闭气泡不会清除结果卡片。

采购／销售保存并过账时，保存完成阶段仅展示进行中的信息卡片，最终只弹出过账结果。保存成功但过账失败显示警告并保留同一草稿；响应丢失时先核实，无法确认时持续展示待核实状态，不宣称成功或确定失败。保存核实确认未提交时显示信息提示，继续沿用既有恢复与重试流程。后续读取失败不覆盖已经确认的业务结果。

普通采购／销售分别说明库存增加／扣减和应付／应收增加。直送、纯服务和零金额分别表达库存或往来余额无变化。草稿保存／取消不宣称业务生效；已过账单据取消说明冲销原变动并保留记录。期初、收付款、退款和往来冲销展示服务端返回的客户／供应商方向及变动前后余额，不推断单张业务单据结清。

库存调整、退货及往来结算沿用既有业务操作和 API；本次不改变事务、审计、数据库结构或接口契约。

## 验证记录（2026-10-02）

- `task frontend:lint`：通过，无 lint 警告。
- `task frontend:build`：通过；保留既有 Browserslist 数据时效和 bundle 大小提示。
- 新增 `npm run test:business-feedback`：隔离 SQLite API 与真实浏览器通过，覆盖采购保存／过账、纯服务零金额销售、两类退货编辑／过账／冲销、库存调整新建／过账／冲销、两个方向的期初／收付款／退款／往来冲销。
- 同一脚本验证成功气泡数量、3 秒及 6 秒自动消失、手动关闭、刷新不重复成功、结果卡片持续可见、弹窗上方的错误气泡、失败保留原因、余额刷新失败仍保留成功及窄屏布局。失败取消响应用浏览器路由注入 HTTP 409；其余业务通过真实 API 执行。
- 原有采购／销售过账脚本通过，覆盖过账丢响应、待核实、原单重试、双人版本变化、真实校验失败及库存不足。原有采购／销售草稿和工作台双账号脚本通过，覆盖保存丢响应、后续其他人过账、输入保留和恢复标识。
- 后端检查、PostgreSQL/SQLite 双库门禁和 Swagger 生成未执行：本次仅修改前端反馈及浏览器测试，不修改后端、migration 或 REST API。

## 复现

仅对隔离测试环境运行浏览器脚本，它会创建真实资料、单据、库存和往来流水。可按现有 `web/tests/phase5-acceptance.md` 启动独立 SQLite API，并为没有本地配置的 worktree 设置隔离环境用 JWT secret。

在 `web/` 执行：

```sh
FEEDBACK_API_URL=http://127.0.0.1:18154 npm run test:business-feedback
PHASE5_API_URL=http://127.0.0.1:18154 node tests/phase5-purchase-post.mjs
PHASE5_API_URL=http://127.0.0.1:18154 node tests/phase5-sale-post.mjs
PHASE5_API_URL=http://127.0.0.1:18154 node tests/phase5-purchase-drafts.mjs
PHASE5_API_URL=http://127.0.0.1:18154 node tests/phase5-sale-drafts.mjs
PHASE5_API_URL=http://127.0.0.1:18154 node tests/phase5-workbench-accounts.mjs
```

反馈脚本默认开发种子账号 `admin` / `admin123`，可用 `FEEDBACK_UI_PASSWORD` 指定密码；已有脚本沿用 `PHASE5_UI_PASSWORD`。可用 `PLAYWRIGHT_EXECUTABLE_PATH` 指定 Chromium。截图默认写入 `/tmp/business-feedback-artifacts`，可用 `FEEDBACK_OUTPUT_DIR` 修改目录。
