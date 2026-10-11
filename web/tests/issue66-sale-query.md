# 销售出库查询与净销售合计真实浏览器验收（#66）

真实浏览器验收销售出库的“按单据/按明细”双视图、跨单明细查询与净销售合计。只使用隔离数据库：脚本通过公开 API 真实创建客户、商品、服务、库存和销售单，其中一张过账销售办理部分退货，并把商品改名、把另一个商品停用，然后使用真实 Chromium 检查页面。

从仓库根目录迁移并启动独立 SQLite API：

```sh
rm -f /tmp/issue66-acceptance.db
APP_DATABASE__URL=/tmp/issue66-acceptance.db task db:migrate:sqlite
APP_CONFIG_PROFILE=sqlite APP_DATABASE__URL=/tmp/issue66-acceptance.db \
APP_HTTP__ADDRESS=127.0.0.1:18109 APP_FILE__STORAGE_ROOT=/tmp/issue66-acceptance-files \
task api:sqlite
```

从 `web/` 执行（脚本自行使用 4197 端口启动 Vite 并在结束时清理）：

```sh
PHASE5_API_URL=http://127.0.0.1:18109 node tests/issue66-sale-query.mjs
```

覆盖内容：

- 筛选字段占满面板宽度，查询与重置在面板底部靠右并与字段右边缘对齐，刷新位于列表工具栏（不是独占侧栏）。
- 本月按北京时间业务日历设置当月起止日期；已过账只设置状态并保留其他条件；重置清空全部条件与页码，视图与排序保留。
- 按单据视图直接显示销售内容摘要，展开显示整单全部明细；商品筛选后仍显示整单并突出匹配商品行。
- 切换到按明细保留筛选条件并回到第一页；明细跨单展示业务日期、客户、商品或服务、数量与单位、成交单价、原金额、有效退货与净销售额。
- 全结果合计按草稿/已过账/已取消分别展示原金额，已过账另有有效退货与净销售额；数量按商品身份与单位、状态分组，服务不显示可退数量。
- 单价/原金额/净销售额排序回到第一页，翻页与每页条数不改变合计，空结果为零合计。
- 从明细打开来源单后返回，视图、筛选、排序与页码保持不变；迟到响应不覆盖更新的查询。

需要安装与 Playwright 对应的 Chromium，或使用 `PLAYWRIGHT_EXECUTABLE_PATH` 指定已有可执行文件。
