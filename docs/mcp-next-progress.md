# MCP 下一阶段实施进度与验证记录

规格：[MCP 后续实施规格](mcp-next-spec.md)｜需求决定：[讨论记录](mcp-next-plan.md)｜架构取舍：[ADR-0017](adr/0017-recoverable-mcp-tokens-and-inherited-query-permissions.md)｜使用说明：[MCP 使用方法](mcp-usage.md)

基线：`45cc1ae`（`feat/private-mcp-v1`，PR #93 首版实现）。开发分支：`feat/mcp-next-token-admin-and-queries`。

## 交付内容

| 工作包 | 交付 |
| --- | --- |
| T01 凭证与授权 | 双库 Goose schema `00031`、seed `00028`；可恢复密文查看；永久／有期限／延期／停用启用／重置；旧摘要令牌兼容；`token:` 与 Bearer 双写法；动态继承用户业务权限；管理 REST 与审计 |
| T02 后台页面 | 系统管理「MCP令牌」菜单与页面：列表、创建、查看复制、延期、启用停用、重置；CLI 与 `task dev:sqlite` 同步 |
| T03 查询工具 | 保留四个工具并扩展字段，新增六个，共十个；日期／状态／净额／双方向／快照语义；往来明细与对账的有界分页 |
| T04 统一验收与交付 | 双库 SQLite 契约、迁移与升级、真实 HTTP／SDK 调用、`task dev:sqlite` 实机流程、前后端检查与文档 |

### 关键实现取舍

- **名称复用 `client_id`**：管理页的「名称」沿用原 `client_id` 列，不新增重复列；`00031` 只增加 `disabled_at`、`secret_version`、`secret_cipher`，并把 `expires_at` 改为可空（永久 = 空到期时间）。SQLite 无法就地去掉 NOT NULL，因此使用整表重建，并在 `server/migrations/layout_test.go` 登记 `mcp_token_new` 例外。
- **显示密钥**：`crypto/hkdf` 以 `jwt.secret` 为输入、独立用途串派生 32 字节密钥，`AES-256-GCM` 加密，密文以 `nonce‖ciphertext` 版本化保存，并将摘要作为附加认证数据绑定到该条凭证。认证仍只用 SHA-256 摘要，所以更换 `jwt.secret` 只影响查看，不影响认证；此时接口明确提示「恢复原配置或重置」，不谎称令牌不存在。
- **权限不再逐枚勾选**：`Permissions` 把工具名映射到业务权限码，每次请求实时解析绑定用户当前的菜单授权；旧 `scopes` 列保留但不再过滤，CLI 的 `--scopes` 仍被接受并打印废弃提示。
- **往来有界读取**：`receivable` 的明细查询改为数据库层 `COUNT` + `LIMIT/OFFSET`，对账改为在同一只读快照内流式累计期初／增加／减少并用 `big.Int` 求和，记录按页返回、合计始终覆盖完整期间。分页偏移做了溢出保护（极大页码返回空页而不是回绕）。REST 对账端点不传分页参数时仍是「全部记录」，打印与导出行为不变。
- **超时与错误映射**：工具错误统一走 `mapError`，区分未认证、无权限、非法参数、对象不存在、超时与暂时不可用；输出超过上限时明确返回 `output_too_large`，不静默截断业务字段。
- **审计落库**：`sys_oper_log` 一直有 `request_params`／`response_result` 两个 TEXT 列，但记录器从未写入。改为在这里落库管理动作的参数与结果，令牌管理因此能回答「谁对哪枚、绑定谁的令牌，把有效期从 X 变成 Y／状态改为停用」，并且从不写入原值或密文。选择复用既有列而不是新增列，是因为仓库既有契约要求**新代码在旧 schema 上仍能写入业务数据**（6 个 upgrade 契约在迁移到中间版本后直接调用 API），新增列会让这些写入整体失败。
- **快照判定以过账时刻为准**：销售单详情的 `snapshotAvailable` 由 `postedAt` 决定，而不是由状态决定——过账前就被取消的草稿从未生成快照，不能把当前客户档案当成冻结值返回。
- **可退化筛选修好**：令牌列表的名称检索改用仓库统一的 `database.LiteralContains`（`ESCAPE '!'` 与 `!`→`!!`），含 `_`／`%` 的名称不再搜不到。

## 独立复核与修复

实现完成后做了一轮独立对抗式复核（四个视角：凭证与生命周期、安全与凭据泄漏、查询语义、测试鉴别力；每条发现再由独立复核者尝试反驳）。16 条发现中确认 10 条（去重后 6 类），逐条处理：

| 类别 | 处理 |
| --- | --- |
| 管理审计未落库目标用户／令牌ID／有效期状态变化 | 记录器改为把动作参数与结果写入既有 `sys_oper_log.request_params`／`response_result`，令牌仓储在事务内写入含前后值的摘要与参数，并断言这些内容可经「操作日志」详情接口读回 |
| 令牌列表名称检索 LIKE 未声明 ESCAPE | 改用 `database.LiteralContains`，新增含 `_`／`%` 名称的检索断言 |
| 过账前取消的草稿被标为已冻结快照 | 快照判定改为 `postedAt`，并新增「过账前取消」用例断言 `snapshotAvailable=false` 且客户名来自当前档案、打印抬头为 `null` |
| 重置未断言保留原有效期 | 补断言：有期限令牌重置后同一到期时刻、永久令牌重置后仍永久 |
| 库存数量、单据备注、往来前后余额与生效时刻无断言 | 分别改为与业务 REST 端点交叉核对数量、断言单据备注（含无备注时为 `null`）、逐行校验「上笔余额＋本笔金额＝本笔余额」不变量与生效时刻落在请求区间内 |
| 审计「不含原值」断言形同虚设 | 改为直接断言落库的 `resource_id` 与 `summary` 内容，并断言其中不含原值前缀 |

被驳回的 6 条中，有两条我仍按更严格的口径调整了行为：服务行的退货量／退货额改为 `null`（与同响应汇总一致，也符合「不能用 0 冒充不适用」），以及把「只给一端日期」的补齐结果放进显式的 `dateCompletion` 字段，避免任何静默补齐。

## 验证记录

### 已通过

| 检查 | 结果 |
| --- | --- |
| `task backend:check`（`go test ./...` + `go vet ./...`） | 通过，无失败用例 |
| SQLite 全量集成契约 `go test -tags=integration ./integration -run '^TestSQLite' -count=1` | 通过，35.5 秒（含既有 API、打印、导出、附件、备份与迁移生命周期） |
| `task frontend:lint` / `task frontend:build` / `node tests/mcp-tokens.mjs` | 全部通过（页面测试覆盖列表、状态与有效期文案、旧版禁用查看、查看原值与复制、重置与停用确认） |
| `task api:docs` | 生成差异为纯新增：`/api/system/mcp/tokens`、`/page`、`/{id}/secret`、`/{id}/extend`、`/{id}/status`、`/{id}/reset`，无删除 |
| `task dev:sqlite` 实机流程 | 迁移至 schema 31／seed 28，API `0.0.0.0:8300` 与前端 `5173` 均正常；后台登录后菜单出现「MCP令牌」 |

新增数据库契约（SQLite 已运行，Taskfile 与 CI 的分库选择器各匹配一次）：

- `TestSQLiteMCPHTTPContract`：凭证头（`token:`／Bearer／同值双写／不同值、重复、格式非法）、Host 与 Origin、Tunnel 转发头、请求体上限、协议协商、十个工具与严格 Schema、关闭时 404。
- `TestSQLiteMCPTokenAdminContract`：菜单与 `system:mcp-token` 授权、永久／天数／自定义日期创建、互斥与非法输入、遮罩列表、独立查看原值并 `no-store`、更换密钥后的明确提示、延期起点（未过期从原到期、已过期从当前）、永久互斥、停用后立即 401 且延期不自动启用、重置旧值立即失效且保留状态、旧摘要令牌继续可用并可重置后查看、旧 scope 不再限制工具、撤权与账号停用时工具集即时变化、无管理权限的登录用户一律 403、审计记录动作且不含原值。
- `TestSQLiteMCPQueryContract`：参考价／联系方式／开票／税号／银行资料与备注、当前档案与过账快照分离、默认本月与部分区间补齐、默认 POSTED 与 DRAFT／CANCELLED／ALL、9 月销售 10 件 10 月有效退回 2 件得到原始 10／有效退货 2／净 8、服务行不可退货、非过账行净额为空、合计覆盖完整筛选范围（`pageSize=1` 仍返回三条状态汇总）、业务日期与生效时刻两种口径、冲销记录保留与前后余额、双方向对账分别返回、对账分页合计不变、非法参数与输出超限。
- `TestSQLiteMCPMigrationUpgrade`：从 schema 30 起保留旧业务行与旧摘要凭证，升级到 31，断言版本与字段（`secret_version`／`disabled_at` 保持空），再降级升级一次。

### 已知限制

- `audit.Event.ResourceID` 在仓库里没有对应的落库列（`sys_oper_log` 只有 `request_params`／`response_result` 两个自由文本列），因此管理动作的目标令牌 ID 以文本形式记录在结果摘要中，而不是一个可单独查询的数字列。本次没有为此新增列，原因见上文「审计落库」。

### 未执行

- **PostgreSQL 契约与 `task db:check`**：本机 WSL 中 Docker 守护进程未运行（`/var/run/docker.sock` 不存在），`sudo` 需要交互密码，测试助手在容器启动失败时直接失败而非跳过，因此本轮没有取得任何 PostgreSQL 结果。Docker 就绪后在仓库根目录按顺序执行：

  ```sh
  task db:contract -- '^TestPostgres(MCPHTTPContract|MCPTokenAdminContract|MCPQueryContract|MCPMigrationUpgrade)$'
  task db:contract -- '^TestPostgres(PartnerStatement|SaleItemQuery|PurchaseItemQuery|ExcelExport|Inventory)'
  task db:check
  ```

  新增的 PostgreSQL 用例名为 `TestPostgresMCPHTTPContract`、`TestPostgresMCPTokenAdminContract`、`TestPostgresMCPQueryContract`、`TestPostgresMCPMigrationUpgrade`，均以 `TestPostgres` 开头，会被 Taskfile 与 CI 的 PostgreSQL 选择器命中（SQLite 侧由 `^TestSQLite` 命中）。CI 的 `database-compatibility.yml` 保留全量双库门禁。

### 待人工验收

- Cherry Studio 与手机实机、以及 Cloudflare Tunnel 公网域名：自动化已覆盖协议协商、转发头与回环来源外部 Host，但实机连接仍需使用者操作，未按已验收处理。步骤见[使用方法](mcp-usage.md)。
