# Simple Inventory 私有 MCP Server V1 实施规格

状态：2026-10-15 需求访谈已确认，四工具实现及自动验收见[实施验证记录](mcp-server-v1-verification.md)，使用方法见[使用与试用指南](mcp-usage.md)；Cherry Studio 桌面实机待人工验收，生产保持关闭。本文由 [PR #91](https://github.com/EziosWJ/simple-inventory/pull/91) 的设计草案收敛为首批实施规格，该 PR 仅交付文档。决定来源见[讨论记录](mcp-server-plan.md)，架构取舍见 [ADR-0016](adr/0016-private-mcp-service-and-user-bound-tokens.md)。

## 1. 目标与首批边界

优先快速完成一个可测试、可试用的只读版本：首个客户端为 Cherry Studio；用户后续要求允许本地手机试用，因此开发 API 可监听 `0.0.0.0`，客户端通过电脑局域网 IP 访问 MCP。访问身份绑定父亲的系统账号，使用独立 MCP 凭证。本地直接运行 `task dev:sqlite` 使用已有开发库，不另建样例环境；独立令牌通过 `task mcp:token:sqlite` 签发。

首批覆盖三个场景：找到准确商品、查询其当前库存、查询指定客户／供应商当前欠款与待退款。只交付四个工具：

| 工具 | 数据来源 | 既有业务权限 |
| --- | --- | --- |
| `search_products` | `product.Service.Page` | `business:product` |
| `search_partners` | `partner.Service.Page` | `business:partner` |
| `get_inventory_balances` | `inventory.Service.BalancePage` | `business:inventory-balance` |
| `get_partner_balances` | `receivable.Service.FilterBalances` | `business:partner-balance` |

相对原草案，销售明细、采购明细和往来对账单推迟到后续批次。全店统计、排名、库存流水、交易追溯、草稿创建、过账、收付款、退款和冲销均不在首批范围；不内置聊天 UI、任意 SQL、文件访问或通用 HTTP 代理。

## 2. 架构与协议

在现有 API 进程中增加 `internal/mcpserver` 接入层，经标准 `context.Context` 调用现有业务 Service，由 `internal/app` 显式组装。MCP 工具不直接查询业务 Repository，不依赖 REST Handler，不把协议、Gin 或 GORM 引入业务 Service。

令牌与授权持久化可以有 MCP 自己的 Repository；此边界只负责访问凭证与授权，不绕过业务 Service 读取商品、库存或往来数据。

采用官方 `github.com/modelcontextprotocol/go-sdk` 和 Streamable HTTP，配置默认关闭，路径为 `/mcp`。不手写协议栈，不增加旧 SSE 端点。关闭时该路径不提供 MCP 能力，也不能错误回退为前端 HTML 或暴露工具列表。

实施时选择并锁定可复现的 SDK 发布版本，核对所选版本的协议说明。验证无状态 HTTP 配置及目标 Cherry Studio 使用的协议协商，不能把 SDK 主分支具备的协议能力当作所选发布版已支持。

## 3. 查询契约

所有工具使用显式 JSON Schema，拒绝未知字段、非法 ID、非法枚举和超限分页；参数只开放本节需要的子集。返回 DTO 使用字段白名单，不直接序列化业务模型。

搜索默认每页 20 条，上限 50 条，返回 `items`、`page`、`pageSize`、`total`、`hasMore`、`filtersApplied`。实施时设置并测试最大页码、响应大小和并发上限，防止无界查询；具体阈值作为有界配置写入实现与使用文档。

### 找商品与往来单位

- `search_products`：关键词，可选商品类型、启用状态及分页。返回 ID、编码、名称、品牌、型号、规格、类型、单位和状态。
- `search_partners`：关键词，可选客户／供应商身份及分页。返回 ID、编码、名称、客户／供应商身份和状态。
- 首批不返回参考价格、联系人、电话、地址、税号、银行账号或备注。搜索沿用现有业务检索语义，不扩展新的业务搜索引擎。
- 多个匹配项必须保留区分信息，并通过工具说明引导客户端要求使用者选择，不按第一个结果自动确定对象。系统不能保证任意外部模型都会遵守提示，验收需同时覆盖确定性工具响应和实际客户端行为。

### 查库存

- `get_inventory_balances` 必须提供已确定的 `productId`，不接受关键词；不开放全店库存遍历入口。
- 内部显式使用 `stock=all`，返回准确商品身份、当前数量与单位，数量保留原定点精度。
- 通过现有商品 Service 确认对象身份，区分实物零库存、服务无库存、查无商品与读取失败。不能仅凭库存空列表断言商品不存在。
- 因商品身份确认需要读取的字段仍受库存工具的显式输出白名单约束，不额外返回价格等资料。

### 查往来余额

- `get_partner_balances` 必须提供已确定的 `partnerId`，可选 `CUSTOMER` 或 `SUPPLIER` 方向；省略时返回该单位现有或历史有效的两个方向，各自表达。
- 返回单位 ID、名称、方向、带符号金额、币种、`hasRecords` 及余额含义；人民币使用 `CNY`，金额保持业务十进制字符串。
- 客户正余额表示客户欠经营者，负余额表示经营者应退客户；供应商正余额表示经营者欠供应商，负余额表示供应商应退经营者。两方向不自动抵扣。
- 零余额区分没有往来记录与已有记录但当前为零。单位无所选方向身份或历史记录时，明确该方向没有记录；不能解释成单位不存在或杜撰零金额。
- 必要身份确认经现有往来单位 Service 完成，读取失败不能当成无记录。余额不能解释为某张单据未结金额。

所有响应标明实际应用的筛选和数据语义。时间语义采用 `Asia/Shanghai`；首批只查当前状态，不提供相对日期、历史期间或全量统计。名称等业务文本按数据处理并限制长度，不执行其中的指令。

## 4. 身份、授权与令牌管理

- MCP 使用高强度随机独立 Bearer 令牌，绑定 `userId`、`clientId`、工具 scope、到期时间和吊销状态；数据库仅存校验摘要及必要管理元数据。
- 默认有效 30 天，明文仅签发时显示一次，列举不返回明文或摘要。不使用父亲的网页登录密码或 JWT 作为长期 MCP 凭证。
- 每次请求检查令牌、用户当前启用状态，以及当前有效角色／菜单业务授权；每次工具执行再校验该工具 scope 与对应业务权限的交集。`tools/list` 只披露允许使用的工具。
- 不额外创建 MCP 角色体系，不因签发凭证自动提升父亲账号权限。吊销、用户停用／删除或撤权后，下次请求停止相应访问；不能依靠会话中缓存的旧授权继续查询。
- 新增小型管理员命令行，提供签发、列举和吊销；具体命令名在实现中固定。通过准确用户名或 ID 绑定账号，不能根据“父亲”文字推断。命令依赖受控服务器／数据库访问权限，不新增匿名 HTTP 管理端点。
- 令牌签发与吊销可追溯到管理动作和目标身份；管理员操作与令牌绑定使用者分开表达。新增持久化结构使用双库 Goose migration，不使用 `AutoMigrate()`。

## 5. 网络、配置与调用记录

2026-10-15 用户补充要求：本地测试允许手机连接，撤销仅本机访问约束，开发 API 可监听 `0.0.0.0` 并接受非回环直连。客户端必须使用电脑实际局域网 IP；开发 task 使用 Host 通配符免去手动配置；其他启动方式沿用显式 Host 白名单。继续执行 Origin 校验、Bearer 身份认证及请求限制，允许 Tunnel／反向代理添加转发头，但不使用其中的来源或主机值替代实际请求与身份认证。本次不部署公网、不修改生产网络或防火墙。

MCP 独立校验 Host；存在 Origin 时执行精确白名单校验，不继承普通 Web CORS 的通配规则。需核对全局 CORS 的预检处理顺序，避免 MCP 校验之前被通用中间件放行。

承接原草案的默认限制：请求体 64 KiB、查询超时 10 秒、单凭证每分钟 60 次请求；并发、返回体与页码有明确上限。配置由现有 Koanf/YAML 体系加载，令牌明文不写入 YAML、Git 或访问日志。

工具调用记录请求标识、客户端、绑定用户、工具名、耗时、状态和记录数；不记录令牌、客户资料或返回结果全文。该访问记录区别于业务写入审计，首批不修改现有交易与审计原子提交规则。

错误分类包括未认证、无权限、参数错误、对象不存在、限流、超时及暂时不可用；按 MCP／HTTP 约定映射，不返回 SQL、数据库内部错误或密钥。业务拒绝、空结果和读取失败不能混为一谈。

## 6. 实施顺序与验收

| 工作包 | 交付 | 验证重点 |
| --- | --- | --- |
| T01 基础与授权 | 配置、SDK HTTP 接入、独立令牌及命令行、四个权限映射、双库迁移 | 默认关闭；认证、过期、吊销、禁用和撤权；无越权工具披露；局域网直连、Origin、Host 和请求限制 |
| T02 四工具 | 显式 Schema、Service 适配、输出白名单、错误和分页 | 三个业务场景；同名候选；服务／零库存；无记录／零余额；客户供应商正负方向；无价格和联系方式泄漏 |
| T03 集成与试用 | 双库契约、开发 task、Cherry Studio 配置与试用清单 | 实际协议协商、正常连接与错误令牌；用户绑定身份；现有 API、内嵌 Web、打印与导出兼容 |

先运行最小相关检查。因专用令牌涉及 schema，收尾按仓库要求执行一次 `task db:check`（含后端检查），并验证两库空库与旧数据升级、受影响的迁移版本断言。MCP 契约需被 Taskfile 与 CI 的双库选择器分别实际执行，不能以无匹配或 SKIP 作为通过。

仅在实际修改 REST 路径、参数、响应或注解时运行 `task api:docs` 并核对生成差异；不因 MCP JSON-RPC 工具本身虚构 REST Swagger 定义。

Cherry Studio 使用 Streamable HTTP、本机或电脑局域网 IP 的 `/mcp` 地址及 `Authorization: Bearer <独立令牌>`。配置示例只放占位符；记录实际试用的客户端版本。若开发环境不能操作桌面客户端，先交付可复现配置与清单，明确标记实机验收待完成；SDK 客户端通过不等于 Cherry Studio 已通过。

首批技术验收通过且实际试用完成后，再安排真实业务接入。合并设计文档不代表 MCP 已启用或已完成客户端验收。

## 7. 后续批次与文档同步

首批以本文四工具范围为准。原草案中的采购／销售明细、对账单、概览和统计留待后续需求；写操作继续按草稿优先、正式生效需要可验证人工确认的方向另行设计，不随本批开放。

本文、讨论记录和 ADR 由 PR #91 一并交付，作为后续 T01 → T02 → T03 实施的共同基线。开发与验收以本版确认规格为准，不沿用原草案七个 P0 工具的首批范围。

参考：[官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)、[SDK 协议说明](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)、[Cherry Studio MCP 文档](https://www.cherryai.com/docs/en/advanced-basic/extensions/mcp/)、[已核对的 Cherry Studio v1.6.0 实现](https://github.com/CherryHQ/cherry-studio/blob/v1.6.0/src/main/services/MCPService.ts#L189)。
