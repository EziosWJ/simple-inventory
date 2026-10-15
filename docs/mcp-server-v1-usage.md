# MCP 使用方法

直接使用现有 SQLite 开发库，无需新建样例库或设置环境文件。

## 启动

在仓库根目录运行：

```sh
task dev:sqlite
```

该命令完成迁移并启动前后端，同时开启 MCP，API 监听 `0.0.0.0:8300`。沿用现有 `server/.data/simple-inventory.db`、账号、权限及业务数据。

## 获取连接令牌

另开终端，在仓库根目录运行一次：

```sh
task mcp:token:sqlite
```

默认绑定本地用户 ID 1（初始管理员）。需要绑定其他已有用户时：

```sh
task mcp:token:sqlite USER_ID=2
```

复制输出 JSON 的 `token` 字段，默认有效30天，启动或重启 API 不需要重新签发。令牌可用工具取用户现有权限与四工具 scope 的交集。

## 客户端配置

Cherry Studio 或支持 MCP 的手机客户端填写：

- 类型：**Streamable HTTP**
- 电脑本机 URL：`http://localhost:8300/mcp`
- 手机 URL：`http://<电脑局域网IP>:8300/mcp`
- 请求头：`Authorization: Bearer <token>`

手机与电脑连接同一局域网；`0.0.0.0` 是监听地址，客户端填写电脑实际 IP。开发 task 已放开 Host，不需要再配置 IP 白名单。若 API 在 WSL 内运行，手机到 WSL 的网络可达性仍取决于本机网络设置。

连接后可用四个工具：找商品、找客户／供应商、按商品 ID 查库存、按单位 ID 查欠款与待退款。同名候选需要明确选择；服务没有库存；客户／供应商两个方向余额分别表达、不自动抵扣。

## HTTPS 域名／Cloudflare Tunnel

Tunnel 服务地址填写 `http://localhost:8300`，MCP 客户端使用 `https://你的域名/mcp`，请求头仍为 `Authorization: Bearer <token>`。本服务允许代理添加的转发头，并支持代理从本机连接时保留外部 Host，无需在 Tunnel 中伪装 localhost Host。

## 令牌管理与其他启动方式

在 `server/` 中可列举和吊销 SQLite 凭证：

```sh
APP_CONFIG_PROFILE=sqlite go run ./cmd/mcp-token list
APP_CONFIG_PROFILE=sqlite go run ./cmd/mcp-token revoke --id <令牌ID> --actor local-dev
```

`task dev:sqlite` 自动开启 MCP；单独运行 API 或其他环境仍默认关闭，可通过 `APP_MCP__ENABLED=true` 开启。监听地址沿用 `APP_HTTP__ADDRESS`，Host 支持 `APP_MCP__ALLOWED_HOSTS='*'` 或逗号分隔的准确 `host:port`。

仍保留令牌认证、现有用户权限和请求限制。四工具只读，不输出参考价格、联系方式、税号、银行账号或备注。技术实现使用官方 Go SDK v1.8.0、无状态 Streamable HTTP，支持2025-03-26、2025-06-18、2025-11-25协商。

Cherry Studio 与手机实机由使用者测试，自动验证结果见[验证记录](mcp-server-v1-verification.md)。
