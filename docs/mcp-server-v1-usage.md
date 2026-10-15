# 私有 MCP V1：使用与隔离试用

本功能默认关闭，在现有 API 进程提供 `/mcp`，只读查询商品、往来单位、当前库存和指定单位当前余额。没有业务写工具、交易明细、对账单、统计、任意 SQL 或文件代理。实现依据见[四工具规格](mcp-server-v1-design.md)及 [ADR-0016](adr/0016-private-mcp-service-and-user-bound-tokens.md)。

## 协议与客户端

锁定官方 Go SDK **v1.8.0**（tag commit `3f3b699b2b67e1ed033a63d6651671dab53c2d32`），使用 `StreamableHTTPOptions{Stateless:true, JSONResponse:true}`，不创建 SSE 旧端点和跨请求授权会话。每个 HTTP 请求生成当前主体允许的工具集合，工具执行前再次校验令牌、账号、scope 和业务权限。

[该发布版协议文档](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/docs/protocol.md)和 `mcp/shared.go` 包含 `2024-11-05`、`2025-03-26`、`2025-06-18`、`2025-11-25`、`2026-07-28`。本服务通过 `SupportedProtocolVersions` 主动收窄为 **2025-03-26 / 2025-06-18 / 2025-11-25**，保留 Cherry 所需 initialize 协商，不开放新版本无握手模型。GET/DELETE 遵循 SDK 无状态模式行为，不依赖 session ID。

[Cherry Studio v1.6.0 源码](https://github.com/CherryHQ/cherry-studio/blob/v1.6.0/src/main/services/MCPService.ts)使用 `streamableHttp`、`baseUrl` 和 `server.headers`，将自定义头合并到 SDK 请求。其 package.json 声明 `@modelcontextprotocol/sdk: ^1.17.5`。本次额外以官方 JS SDK **1.17.5** 运行真实连接 smoke，确认 JSON 响应、`Authorization`、`Accept: application/json, text/event-stream`、后续 `MCP-Protocol-Version: 2025-06-18` 和无会话 GET 行为可用。这不是桌面 Cherry Studio 已验收的结论。

## 创建独立样例

在仓库根目录运行：

```sh
MCP_DEMO_HOST=192.168.1.10 scripts/mcp-demo.sh
```

将 `192.168.1.10` 换成运行 API 的电脑局域网 IP。脚本监听 `0.0.0.0:18300`，并将此 IP 加入 MCP Host 白名单；不填 `MCP_DEMO_HOST` 时白名单只包含本机地址，手机访问前需添加电脑 IP。MCP 与 API 共用监听端口，因此该配置也用于隔离 API 和内嵌 Web。

需要 Go、Python 3、OpenSSL。脚本固定创建新的 `/tmp/simple-inventory-mcp-demo.XXXXXX` 私有目录，不接受数据库路径，不读取现有业务库。Goose 执行完整 SQLite 迁移；创建 `father-mcp-test` 对应测试身份，其密码不可用于 Web 登录；分配现有四个业务 permission_code。脚本输出准确 `userId` 和 `env.sh` 路径。

样例是人工构造的只读夹具（包括直接写入的余额状态），不是业务导入或过账操作范例：

| 对象 | 预期 |
| --- | --- |
| DEMO-G1 / 同名硒鼓 / A型 | 12.5 支 |
| DEMO-G2 / 同名硒鼓 / B型 | 0 支 |
| DEMO-S / 安装服务 | 不适用库存，quantity=null |
| DEMO-P1 / 双向往来样例 | 客户欠 123.45；供应商应退 67.89，各自保留 |
| DEMO-P2 / 无往来样例 | 两个当前身份均无记录，hasRecords=false |

进入 `server/`，使用脚本输出的真实路径和 userId：

```sh
source /tmp/simple-inventory-mcp-demo.XXXXXX/env.sh
go run ./cmd/mcp-token issue --user-id <测试身份ID> \
  --client cherry-studio-trial --actor <本次管理员标识> \
  --scopes search_products,search_partners,get_inventory_balances,get_partner_balances
go run ./cmd/api
```

签发输出 JSON 的 `token` 只显示这一次。不要把它粘贴进命令、仓库、报告或共享日志；直接填入 Cherry 的本地请求头设置。父亲真实账号的 ID 必须在正式业务接入时另行核对，本脚本不会推断、修改或复制生产账号。

停止试用时 Ctrl-C 停止 API，吊销试用令牌；临时数据可在人工确认不再需要后删除。不要将样例库用于真实业务。

## 管理员令牌 CLI

CLI 使用现有 Koanf 配置加载及受控主机／数据库访问权限，不提供 HTTP 管理端点，也不以填写 `--actor` 代替主机权限。`--actor` 是可追溯的管理员操作标识，与目标 `--user-id` 分开持久化。管理员不能通过签发操作提升用户权限。

```sh
# 在 server/ 且已加载目标隔离环境时运行
go run ./cmd/mcp-token list --user-id <准确ID>
go run ./cmd/mcp-token revoke --id <令牌ID> --actor <管理员标识>
```

- `issue` 必填准确正整数用户 ID、client、actor 和至少一个合法工具 scope。
- 默认有效期 720h（30天），可用 `--ttl` 调整，限制为大于零且不超过2160h（90天）。
- 原始令牌为 32 字节密码学随机数，前缀 `simcp_`；数据库只存 SHA-256 摘要和管理元数据。
- `list` 可省略 user-id 列出全部管理元数据，不输出原文或摘要。
- `revoke` 持久化时间和操作者；不存在或已吊销返回错误。
- 目标用户删除／停用、角色删除／停用、菜单删除／停用或解除角色菜单绑定、scope 不足，都在下次请求生效。`tools/list` 和 `tools/call` 使用相同交集。
- 发布构建生成 `simple-inventory-mcp-token`；容器镜像含 `/app/mcp-token`。这不代表本次已部署或已启用。

## Cherry Studio 连接设置

在电脑或同一局域网的手机 MCP 客户端中新增服务（手机必须支持 Streamable HTTP 和自定义请求头）：

- 名称：`Simple Inventory 隔离试用`
- 类型：**Streamable HTTP**（内部类型 `streamableHttp`）
- URL：电脑本机用 `http://127.0.0.1:18300/mcp`，手机用 `http://<电脑局域网IP>:18300/mcp`
- 请求头：`Authorization` = `Bearer <签发时显示的独立令牌>`
- 无需填写 Web 登录密码、JWT、OAuth 或命令行启动参数。

字段示意（按目标版本的界面填写，不保证其导入格式完全相同）：

```json
{
  "name": "Simple Inventory 隔离试用",
  "type": "streamableHttp",
  "baseUrl": "http://127.0.0.1:18300/mcp",
  "headers": { "Authorization": "Bearer <MCP_TOKEN>" }
}
```

Electron 主进程通常没有 Origin。如果实际请求带 Origin，先确认准确值，再添加到独立 MCP 白名单；不要填 `*`。手机上的 `127.0.0.1` 指手机自身，不能填它；也不要把监听地址 `0.0.0.0` 填作连接地址。若在 WSL 中运行，手机到 WSL 的网络可达性及电脑防火墙需在目标机器核对，本次不修改防火墙、路由或生产网络。

## 配置和访问边界

沿用 `APP_` 环境变量、`__` 分层与现有 YAML 配置；未配置 `mcp.enabled` 时为 false。MCP 不再强制回环监听和回环来源，可使用 `APP_HTTP__ADDRESS=0.0.0.0:18300` 供手机直连。默认仍关闭；通用 API 监听默认值不因本功能改变。Host 白名单必须配置客户端连接 URL 中的准确主机与端口。

| YAML 键（mcp 下） | 默认值 / 范围 |
| --- | --- |
| enabled | false |
| allowed_hosts | `127.0.0.1:18300`、`localhost:18300`、`[::1]:18300`，精确 authority 含端口 |
| allowed_origins | 空列表；无 Origin 可访问，有 Origin 必须精确匹配 |
| max_body_bytes | 65536；1..65536 |
| max_output_bytes | 131072；1024..1048576，限制最终 HTTP 响应及单工具结果 |
| timeout | 10s；大于0且不超过10s，含认证与查询；设置真实连接读 deadline |
| requests_per_minute | 60；1..600，进程内每凭证固定一分钟窗口 |
| max_concurrent | 4；1..32，整个 MCP 入口的全局并发槽 |
| max_page | 1000；1..1000，搜索每页默认20，上限50 |

例如 `APP_MCP__ALLOWED_HOSTS=127.0.0.1:18300,localhost:18300,192.168.1.10:18300`。每个 HTTP 请求只允许一条 JSON-RPC 消息，拒绝旧协议 batch，避免批量扇出绕过频率／并发限制。并发占用期间超时不会提前释放仍在执行的槽，防止慢依赖无限堆积；频率表最多10000个有效窗口，到期清理，重启进程清空。

允许真实非回环连接来源；不信任代理头，存在 Forwarded、X-Forwarded-For、X-Forwarded-Host 或 X-Real-IP 时拒绝。拒绝查询串传参。Origin、Host 校验在全局 Web CORS 预检之前执行。禁用时 `/mcp` 和子路径返回404，不回退到 SPA。响应 `Cache-Control: no-store`；工具列表 MCP 缓存提示为 private、TTL 0。

## 工具参数与结果

| 工具 | 可接受参数 |
| --- | --- |
| search_products | keyword（最多200字符）、type=GOODS/SERVICE、status=0/1、page、pageSize |
| search_partners | keyword、identity=CUSTOMER/SUPPLIER、status、page、pageSize |
| get_inventory_balances | 必填 productId，必须先确定准确内部 ID |
| get_partner_balances | 必填 partnerId，可选 direction=CUSTOMER/SUPPLIER |

Schema 拒绝未知字段、null、非法枚举、非整数或超出安全整数范围的 ID、超限页码。搜索遵循原业务 Service 检索语义，响应包含分页、filtersApplied 和 selectionRequired；名称等文本最多200字符。多个候选始终保留并提示选择，外部模型是否遵守仍需试用。

商品仅返回 ID、编码、名称、品牌、型号、规格、类型、单位、状态；往来单位仅返回 ID、编码、名称、客户／供应商身份、状态。没有参考价格、联系方式、地址、税号、银行信息、备注或交易详情。

库存显式使用 `stock=all`。实物无余额记录沿用业务 Service 的零数量字符串，服务 `quantity=null`、`inventoryStatus=NOT_APPLICABLE_SERVICE`。商品不存在返回 not_found；读取失败不能当零库存。

往来金额保持业务定点十进制字符串、CNY、hasRecords 和方向：客户正值=客户欠经营者，负值=经营者应退客户；供应商正值=经营者欠供应商，负值=供应商应退经营者。不计算净额，不解释单张单据的结清状态。已有方向无记录可返回0及 hasRecords=false；已有记录的零余额为 hasRecords=true。明确请求不存在的方向时 amount=null、hasRecords=false，并说明无所选方向身份或历史记录；省略方向只返回当前或历史有效方向。

HTTP 错误包括401认证失败、403访问边界、413请求体过大、429频率／并发限制、503超时／暂不可用／最终输出过大；工具执行错误以 `isError=true` 和稳定 error 字段表达 invalid_arguments、not_found、forbidden、timeout、temporarily_unavailable 或 output_too_large。未知／未授权工具遵循 SDK 的 JSON-RPC 错误。错误不输出 SQL 或密钥。

工具访问日志只记录服务器生成的请求 ID、client、user_id、tool、状态、耗时、记录数，不记录原始令牌、输入对象名称或结果全文；不写业务交易审计、不改变原业务事务。

## 逐步人工试用清单

桌面客户端版本：**待填写**。以下均为 Cherry Studio 实机验收待完成项：

- [ ] 记录操作系统、Cherry 版本、API commit、样例目录；连接后仅出现四工具；手机通过电脑局域网 IP 连接。
- [ ] 输入“找同名硒鼓”，返回A/B两个候选，要求用户选择；不能默认第一项。
- [ ] 指定 DEMO-G1 查询12.5支；DEMO-G2为0；服务无库存；不存在 ID 明确报错。
- [ ] 搜索“双向往来样例”，客户欠123.45、供应商应退67.89，分别陈述且不抵扣；无往来样例 hasRecords=false。
- [ ] 改用错误令牌连接失败；恢复令牌后恢复查询。
- [ ] 用 CLI 吊销后同一客户端下次调用失败；只在隔离账号测试停用与撤权。
- [ ] 返回结果不含价格、联系方式等禁用字段；模型不把业务文本当指令。
- [ ] 保存不含凭据的结果记录，停止隔离 API；不接入生产数据。

可选复现 JS SDK 验收（需要 Node/npm；依赖仅安装到 `/tmp`）：

```sh
npm install --prefix /tmp/mcp-js-client --ignore-scripts --no-audit --no-fund @modelcontextprotocol/sdk@1.17.5
# 凭据文件手工保存 CLI 本次签发 JSON，放在私有目录、权限600；不要重新签发已显示过的原文。
node scripts/mcp-client-smoke.mjs /tmp/<私有目录>/credential.json
```

验证记录见 [MCP V1 验证记录](mcp-server-v1-verification.md)。
