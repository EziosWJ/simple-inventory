# HTTP 环境采购保存失败排查

日期：2026-10-08。

客户反馈：生产地址 `jxc.ezios.cn`，采购商品为复印机墨盒，供应商王孝永，数量 1、单价 200 元。截图地址栏显示非安全连接。实际截图中的资料名称为“复印机粉盒”和“永乾科技（王孝永）”。

## 证据与原因

- 只读访问 HTTP 首页返回 200；HTTPS 在本次访问环境中发生 TLS 握手错误。
- 本地真实 Chromium 的普通 HTTP 域名环境：`isSecureContext=false`、`crypto.randomUUID` 未定义，`crypto.getRandomValues` 可用。
- 修复前运行 `cd web && node tests/http-business-save.mjs`，点击采购保存后报 `crypto.randomUUID is not a function`，保存请求次数为 0。
- 采购页在进入请求异常处理之前直接调用 `crypto.randomUUID()`。因此 HTTP 环境会中断保存，页面不能展示接口错误。商品、供应商及金额不影响这一故障的触发。
- SQLite 采购草稿和幂等保存 HTTP contract 通过。PostgreSQL 定向检查因 Docker daemon 权限不可用而跳过。

## 修复

`web/src/lib/uuid.ts` 在可用时使用浏览器原生 `randomUUID`，否则用 `getRandomValues` 生成 UUID v4，继续保留随机请求键与幂等语义。采购、销售、收付款、期初往来及附件统一使用该函数。

HTTP 浏览器回归完全拦截 API，不写生产数据；验证保存请求、拒绝后保留输入、重试使用不同请求键、保存失败不继续过账，以及销售保存和往来页面加载。

## 验证结果

- `node tests/http-business-save.mjs`：修复前失败，修复后通过。
- `task frontend:lint`、`task frontend:build`：通过。
- SQLite 定向命令：`GOCACHE=/tmp/simple-inventory-go-cache go test -tags=integration ./integration -run '^TestSQLite(Phase5PurchaseSaveHTTPContract|PurchaseDraftPostgresSQLiteHTTPContract)$' -count=1`，通过。
- 对应 `TestPostgres` 定向命令：Docker daemon 权限不可用，两个测试跳过。
- `node tests/business-ui.mjs`：1440/1024 两组页面遍历完成，随后在第 175 行等待销售详情中的 `PI-UI-001` 跳转按钮超时；该用例位于任务开始时已有改动的文件中，未修改该文件。全量浏览器回归未通过。
- 没有修改后端、API 或数据库结构，未运行全量 `task db:check`、`task backend:check`。

## 生产发布

用户提供 `ubuntu` SSH 入口后，明确授权“直接本地构建然后部署新包测试”。真实生产 IP 为 `82.157.189.251`；本地代理 DNS 返回保留地址，SSH 使用真实 IP 和 `KexAlgorithms=curve25519-sha256` 后正常连接。

- 原镜像：`simple-inventory:host-20261003-1`。
- 新镜像：`simple-inventory:http-uuid-20261008-1`，线上容器 image ID 为 `sha256:598209e8a167cd97bdbcbd9ba7e9e6c1f68e3ae927f7a6b9247fc940acb8118a`。
- 发布时间：2026-10-08 21:16（北京时间），部署目录 `/opt/simple-inventory`，systemd 管理 Docker Compose。
- 发布基线：后端 `cmd/internal/migrations/docs` 汇总哈希全部与线上一致；前端在本地 HEAD 基础上保留已上线的 `sales.tsx` 工作区改动后，整个 `web/src` 基线汇总哈希与线上一致。随后只加入本次 UUID 修复，没有发布其他工作区改动。
- 备份：服务器本地 `/opt/simple-inventory/backups/http-uuid-20261008-1/` 保留 `jxc.dump`、原 `.env`、Compose 和旧镜像 ID；`pg_restore --list` 核验通过。没有导出业务数据库或生产配置。
- 没有运行 migration up；发布前后 `migrate check --kind all` 都通过（schema 26、seed 23）。
- 最终容器状态 `healthy`、systemd `active`，外部反向代理首页和 `/ready` 正常。
- 线上 JS 为 `/assets/index-f5LRtJ_x.js`，SHA256 `2ebce882f4b368a887028f6ddf631618f119a78462ebc17f63a6b382781380a2`，与本地实际测试构建包一致。
- 使用 `HTTP_ASSET_DIR=$PWD/dist node tests/http-business-save.mjs` 验证该构建包在非安全 HTTP 下的采购保存、重试、保存并过账、销售保存及往来页面，通过。API 全部拦截，没有替客户创建或过账实际采购单。

生产源码下载曾被自动审批拒绝，因此改用文件哈希核对和本地源码构建；只交换哈希元数据，没有导出生产内部源码。

若需回滚，本次没有数据库结构变更，可将上述备份的 `env.before` 恢复为部署目录 `.env`，再重启 `simple-inventory` 服务使用保留的旧镜像。浏览器需刷新页面取得新 JS；客户实际采购单保存仍由客户复核操作。

## 其他表单同类问题检查

2026-10-08 发布后按用户要求继续检查。全量搜索 `web/src` 中的 `crypto`、`randomUUID`、剪贴板、文件选择器、Service Worker、凭据 API 等调用，仅发现共用 UUID 函数中的受保护原生调用。没有发现表单绕过该函数直接使用 `crypto.randomUUID()`，也未发现其他表单使用需要安全上下文的浏览器 API。

测试环境为真实 Chromium 的普通 HTTP 域名，明确断言 `isSecureContext=false` 且 `crypto.randomUUID` 未定义。使用与线上 SHA256 相同的生产构建包，所有 API 请求被测试拦截，不写生产数据。

| 检查对象 | 浏览器验证结果 |
| --- | --- |
| 采购新建、草稿编辑、保存重试、保存并过账入口 | 通过；能生成请求，保存拒绝不继续过账，输入保留 |
| 销售新建与草稿编辑 | 通过；能发送 POST/PUT 保存请求 |
| 商品、往来单位、库存调整、采购退货、销售退货编辑 | 通过；保存请求正常，拒绝后保留输入 |
| 仓库与经营者打印资料 | 通过；保存请求包含修改后的内容 |
| 客户收款、供应商付款 | 通过；提交、失败重试、成功提示和表单重置正常 |
| 向客户退款、收到供应商退款 | 通过；提交、失败重试、成功提示和表单重置正常 |
| 期初应收、期初应付 | 通过；提交、失败重试、成功提示和表单重置正常 |
| 上述六类往来记录的附件 | 通过；上传失败重试、绑定、图片/PDF 预览、下载、移除历史、补传和冲销后只读 |
| 系统管理、个人资料与修改密码等其余表单 | 代码扫描未发现同类依赖；没有逐项执行实际 CRUD 验收 |

复跑入口（在 `web` 下）：

```sh
HTTP_ASSET_DIR="$PWD/dist" node tests/http-business-save.mjs
HTTP_TEST=1 HTTP_ASSET_DIR="$PWD/dist" node tests/partner-attachments.mjs
```

本轮补充测试和记录，没有新增产品代码改动，无需重新部署。两组定向浏览器测试、`task frontend:lint`、`task frontend:build` 和 `git diff --check` 均通过。此前完整 `business-ui.mjs` 的销售详情跳转超时没有在本轮重新运行；未运行后端或双库全量门禁。本轮结论限定为没有发现剩余的同类 HTTP 前端提交故障，不代表各表单的真实后端持久化与业务事务已再次全量验收。
