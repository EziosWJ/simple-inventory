# Simple Inventory 私有 MCP Server V1 设计规范

> 状态：Draft / 待评审。仅讨论设计，不代表已经实现。
>
> 范围：`EziosWJ/simple-inventory`（Go + Gin 模块化单体），2026-10-13。
>
> 决策：**MCP 进程内集成，直接调用现有业务 Service；V1 只读、默认关闭、私有访问。**

## 1. 背景与目标

项目已在真实店铺投入使用。希望使 ChatGPT、Claude、Codex 等支持 MCP 的 Agent 能以自然语言查询进销存数据，并在后续版本执行经过授权、可确认、可追溯的业务操作。

V1 验收场景：

- 查指定商品的资料、库存，以及采购/销售明细；
- 按业务日期筛选销售、采购，并正确区分草稿、已过账、已取消；
- 查询客户应收、供应商应付及指定往来单位的对账单；
- 在数据不充分时明确说明不足，不通过模型猜测补齐总额；
- 未获授权的调用不能读取任何业务数据。

非目标：

- 不内置聊天 UI，不绑定模型供应商，不做 RAG/向量库；
- 不向 Agent 提供任意 SQL、数据库连接、文件系统或通用 HTTP 代理；
- V1 不支持创建、修改、删除、过账、结算、退款、冲销等写操作；
- 不承诺所有 MCP 客户端支持相同的认证/传输；接入能力需要实测。

## 2. 当前仓库依据（main，2026-10-13 核对）

- `server/internal/app/app.go`：`app.Dependencies` 已注入 `Product`、`Partner`、`Inventory`、`Purchase`、`Sale`、`Receivable` 等 Service；Gin 统一挂载 `/api/v1`。
- `server/internal/auth/{middleware,service,context,permission}.go`：现有 Bearer JWT、服务端登录会话校验、`Principal` 上下文及 `HasPermission`。JWT 主要供 Web 登录使用。
- `server/internal/{product,partner,inventory,purchase,sale,receivable}/`：已有分页/明细/往来账查询 Service 与 Repository。
- `server/internal/receivable/statement.go`：`Statement` 要求 `partnerId`、`direction`、RFC3339 `from/to`，返回期初、增加、减少、期末余额。
- `server/internal/receivable/balances.go`：已有 `FilterBalances` 与 `Overview`。
- `server/internal/{sale,purchase}/service.go`：草稿保存已有 `requestKey` 幂等机制及过账版本校验；可供后续写工具复用。
- `server/internal/audit/` 与 `docs/adr/0006-atomic-operation-audit.md`：业务写入与操作审计在同一事务提交。
- `server/internal/platform/http/middleware.go`：通用 CORS 在 `AllowedOrigins` 为空时允许任意 Origin，因此 **MCP 必须额外执行独立、严格的 Origin 校验**。
- 实际抽查的商品、销售、采购、库存、往来 Handler 中未发现覆盖所有动作的统一权限拦截；`HasPermission` 的存在**不等于**现有业务 API 都已完成后端细粒度授权。

以上为代码现状；新增授权模型、汇总服务及 MCP 配置均需实施，不应当作已具备能力。

## 3. 总体架构

```text
Agent / MCP Client
        |
        | Streamable HTTP + HTTPS（内网可经 VPN/Tunnel）
        v
[Go 进程 /mcp]
  ├─ 私有凭证认证、校验 Origin / Host、限流
  ├─ 身份映射（principal：userId、clientId、scope）
  ├─ 工具白名单与业务权限校验
  ├─ 参数解析、字段脱敏、输出约束、调用记录
  └─ 业务 Tool Handler
         |
         v
现有业务 Service（不依赖 MCP 协议）
         |
         v
现有 Repository → PostgreSQL / SQLite
```

优先新增目录：

```text
server/internal/mcpserver/
  server.go         # SDK 初始化、Tool 注册
  transport.go      # Gin/net/http 对接
  auth.go           # 私有凭证认证、主体解析
  policy.go         # scope + 业务权限校验
  query_tools.go    # 只读 Tool Handler
  dto.go            # Tool JSON Schema / 输出结构
  errors.go         # 面向 Agent 的错误分类
  audit.go          # MCP 访问记录
  *_test.go
```

`app.Dependencies` 可新增可选的 `MCP` 依赖（或配置/装配入口），在 `app.New` 中仅当 `mcp.enabled=true` 时挂载 `/mcp`。MCP 调用通过 Go `context.Context` 传递经过认证的主体，不得直接调用 Repository 或绕过业务 Service。读查询必须统一纳入授权层，即使原有 REST API 未覆盖相同权限，也不能继承这一缺口。

技术选型：`github.com/modelcontextprotocol/go-sdk` 官方 Go SDK；HTTP 使用 Streamable HTTP。优先验证无状态配置与 MCP 2026-07-28 版本兼容性，并进行旧客户端协商测试；SDK 升级应锁定可复现版本。不得手写 MCP JSON-RPC 协议栈。

## 4. V1 Tool 清单与契约

### P0：首批只读工具

| Tool 名称 | 来源 Service | 必要/主要入参 | 业务结果 |
| --- | --- | --- | --- |
| `search_products` | `product.Service.Page` | keyword、type、category、status、page | 商品候选、准确 productId |
| `search_partners` | `partner.Service.Page` | keyword、page | 往来单位候选、准确 partnerId；不默认返回电话/地址 |
| `get_inventory_balances` | `inventory.Service.BalancePage` | productId 或 keyword、stock、page | 库存结存、单位、分页总数 |
| `query_sale_items` | `sale.Service.ItemPage` | businessFrom、businessTo、status、partnerId/productId、page | 销售明细，不把分页金额误称全店销售总额 |
| `query_purchase_items` | `purchase.Service.ItemPage` | businessFrom、businessTo、status、partnerId/productId、page | 采购明细 |
| `get_partner_balances` | `receivable.Service.FilterBalances` | partnerId 或 partnerKeyword、direction、category、page | 客户应收/供应商应付当前余额 |
| `get_partner_statement` | `receivable.Service.Statement` | partnerId、direction、from、to | 指定期间期初、发生、期末余额及受控明细 |

### P1：补充查询工具

- `get_partner_overview` → `receivable.Service.Overview`：全店往来概览及 Top N；
- `get_inventory_entries` → `inventory.Service.EntryPage`：库存流水溯源；
- `query_sales` → `sale.Service.Page`：销售单状态与单号查询；
- `query_purchases` → `purchase.Service.Page`：采购单状态与单号查询。

**统一规则：**

1. Tool 参数使用明确的 JSON Schema：拒绝未知字段；互斥参数（如 ID/keyword）严格校验；日期范围必须合理。对现有 Service 的 Filter 进行薄封装，不暴露全部内部字段。
2. 默认 `page=1, pageSize=20`，MCP 层最多允许每页 50 条；设置总返回大小、查询时限、日期跨度及最大页码，禁止无界遍历。历史对账单底层接口如无法限量，必须补充安全的分页/分段查询后才能开放。
3. 金额必须使用原业务十进制字符串，数量沿用原业务定点精度；不得转换成浮点数后汇总。输出附带 `currency`（配置明确后方可赋值）、`timezone`、查询过滤条件和数据语义。
4. 日期类型区分：业务日期 `YYYY-MM-DD`；实际生效时间 `RFC3339`、明确 `from` 闭区间与 `to` 开区间。相对日期（今天/昨天）按配置的店铺时区解释，不猜测用户/服务器时区。
5. 结果建议统一含 `items`、`page`、`pageSize`、`total`、`hasMore`、`filtersApplied`；对账单另外返回 `openingAmount`、`increaseAmount`、`decreaseAmount`、`closingAmount` 与方向。
6. 敏感信息按最小必要集返回；限制备注、客户名称等非可信业务文本的输出大小，并将其视为数据，不执行其中的模型指令。
7. 统一错误语义：`UNAUTHENTICATED`、`FORBIDDEN`、`INVALID_ARGUMENT`、`NOT_FOUND`、`CONFLICT`、`RATE_LIMITED`、`TEMPORARILY_UNAVAILABLE`。业务细节可读，但数据库错误、SQL、服务器密钥不可暴露。
8. 涉及“今日销售总额”“销售排名”等完整统计时，需要新增**服务端有明确定义的聚合读 Service**；禁止用单页明细推算全店总金额。聚合口径要说明已过账/退货/取消与直送单是否计入。

## 5. 身份认证与权限

V1 面向自用私有部署，采用最小可控方案：

- MCP 开关默认关闭；首次部署仅允许 loopback / 内网 / VPN 访问，不能因开启业务 Web 访问而自动公开 `/mcp`。
- 为 MCP 创建可撤销、可过期、随机生成的**独立访问令牌**（数据库只存校验摘要），绑定 `userId`、`clientId`、允许的 Tool scope、有效期、吊销状态；不要复用长期有效的 Web 登录口令，不向模型上下文暴露令牌。
- 每个请求先验证凭证、用户当前启用状态、工具 scope，再校验与原有 RBAC 映射的业务数据权限。默认拒绝；`tools/list` 也只披露当前主体可用工具。
- 原 `auth.HasPermission` 可作数据库侧授权基础，但应明确新的 `mcp.inventory.read`、`mcp.sale.read`、`mcp.purchase.read`、`mcp.partner.read` 等权限映射。**不得只依赖前端菜单显示/隐藏来决定权限**。
- `Origin` 存在时必须与 MCP 单独配置的精确白名单匹配；不允许因普通 Web CORS 空列表或 `*` 而放行。校验反向代理的 Host / Forwarded Headers，限制请求体、并发和 QPS。
- 读调用记录 `requestId`、`clientId`、`userId`、`toolName`、耗时、结果状态和返回记录数；不要记录令牌、完整客户信息或原始敏感查询结果。

**公网/云端 Agent 阶段另设发布门槛**：按客户端能力实现规范的 OAuth、受众/资源绑定、受保护资源元数据及 HTTPS；不把“内网静态 Token 能用”视为公网认证方案。不支持 OAuth 的客户端只能通过受控本地/代理环境接入，具体以客户端互操作测试为准。

## 6. 配置与部署

建议在现有 Koanf/YAML 配置体系中新增（名称为拟议值）：

```yaml
mcp:
  enabled: false
  path: /mcp
  transport: streamable_http
  access_mode: private
  allowed_origins: []
  max_page_size: 50
  max_request_bytes: 65536
  query_timeout: 10s
  rate_limit_per_minute: 60
  store_timezone: Asia/Shanghai  # 示例：必须按真实店铺配置确认
```

令牌摘要、过期时间与用户关联持久化需新增 PostgreSQL + SQLite 双方言 Goose migration；私钥/令牌明文不写入 YAML 或 Git。配置由现有加载器统一管理。部署时 Caddy 可对 `/mcp` 设置额外的网络访问边界与请求限制；避免把服务映射成一个未经认证的公网端点。

Go SDK 关于 2026-07-28 的提示：Streamable HTTP 要协商该协议版本需设置 `StreamableHTTPOptions.Stateless=true`；上线前通过互操作测试确认该配置对实际客户端的影响。

## 7. 后续写操作路线（V1 不开放）

第二阶段：`create_sale_draft`、`create_purchase_draft`。只新增**草稿**，复用现有 `Create(ctx, audit.Metadata, Input)`；调用应有稳定 `requestKey`，超时先查询原保存结果，不盲目重试新键。用户、商品、单位、数量、金额必须经过原 Service 校验。

第三阶段：过账、收付款、退款、冲销。

- 采用 `prepare -> 人工确认 -> execute` 两步式协议；服务端保管有 TTL、单次使用、绑定主体/请求哈希的确认票据，执行前重新校验权限、状态和单据版本；
- 不能只相信模型生成的“用户已同意”字符串作为确认；
- `audit.Metadata.ActorID` 必须来源于认证主体，绑定 `clientId/toolName/requestId` 可追踪；
- 严格复用现有业务 Service 和事务审计，不允许生成 SQL 写入业务表。

## 8. 测试矩阵与上线验收

### 单元测试

- Schema：缺参、非法枚举、非法日期/精度、冲突筛选条件、超大分页；
- Policy：未认证、过期/吊销令牌、禁用用户、无权限工具、跨主体访问；
- 输出：空结果、分页与总数、金额精度、脱敏、异常映射；
- 安全：无效 Origin、恶意备注/prompt injection、超时、并发与频率限制；
- SDK：Tools 列举、正常执行、协议版本协商、Streamable HTTP 无状态模式。

### 集成/契约测试

- SQLite 与 PostgreSQL 同样通过；
- 使用种子数据逐项对比 MCP Tool 与同条件现有业务查询的结果；
- 客户/供应商方向分别验证，冲销、退款、期初余额不被错误抵扣；
- 单页结果不足以计算全量统计时显式拒绝编造；
- 关闭 MCP 开关时 `/mcp` 不可访问，现有 `/api/v1`、`embed` Web、打印与导出功能行为不回归；
- 使用至少一个目标 Agent 客户端进行端到端连接和鉴权测试。

### 发布验收（必须全部满足）

1. 默认不开启 MCP；无凭证无法访问业务信息。
2. 所有 P0 Tool 都能在授权用户上下文中得到准确业务结果。
3. 禁用用户/吊销令牌之后的下一次请求不能继续访问。
4. 无越权、任意 SQL、隐式写操作或不限量数据导出通道。
5. 对现有生产服务通过回归测试且保留可回滚发布方式。

## 9. 实施拆分（建议 issue）

1. **MCP 基础框架**：配置开关、SDK Handler、协议探针、Origin 校验、禁用态测试；
2. **MCP 身份授权**：令牌迁移、管理/吊销方式、用户身份映射、scope/RBAC 策略；
3. **只读查询 P0**：DTO/JSON Schema、七个 Tool、分页限制、金额与时区定义；
4. **测试与部署**：双数据库集成、Agent 端到端、Caddy/内网部署指引、文档；
5. **P1 查询与服务端统计**：补充其他查询，明确销售/采购汇总口径。

## 10. 评审中待确认

- 私有 MCP 第一批要接入的客户端（本地 Codex / Claude Code / ChatGPT 等），其认证能力与网络可达性；
- MCP 的可访问网络范围（仅店铺局域网、VPN、还是需要公网 OAuth）；
- 店铺业务时区和币种，及“今日销售额”是否扣除销售退货；
- 独立 MCP 权限采用专用角色还是主体级 scope 与现有权限联合；
- 客户档案中哪些字段必须对 Agent 可见。

**参考**：

- [MCP 官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [MCP Streamable HTTP 传输规范](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [Go SDK Protocol Notes](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)
