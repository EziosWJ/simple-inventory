# 验证指南

验证范围以根目录 [CLAUDE.md](../../CLAUDE.md#验证分层)为准。本文提供命令和优化背景。

## 定向选择契约

先查看测试名及其实际覆盖的场景，列出本次改动影响的成功、失败回滚、关联业务及并发行为。按行为选择测试，不能只按业务模块名前缀选择；例如销售变更也可能涉及直送、退货、零金额和往来余额契约。

```sh
rg -n '^func Test' server/integration
# 先列出匹配项，不执行测试；使用 Taskfile 相同的构建缓存。
cd server
GOCACHE="$PWD/../.task/go-build" go test -tags=integration ./integration -list '^Test(SQLite|Postgres)SaleItemQueryHTTPContract$'
```

在仓库根目录按顺序执行：

```sh
task db:contract -- '^TestSQLiteSaleItemQueryHTTPContract$'
task db:contract -- '^TestPostgresSaleItemQueryHTTPContract$'
task backend:check
```

`db:contract` 的参数是 Go 测试名正则，不带 `-run`；使用 `$` 结尾可避免单用例匹配到其他长名称。多契约合并成一个正则，在每种数据库上各运行一次，减少 task/go 启动开销。普通 Go 单元测试保留缓存；契约使用 `-count=1`，强制实际执行。

改动涉及多个模块时扩大定向范围；公共数据库能力、migration 或影响范围不明确时运行 `task db:check`。SQLite 全量可单独运行 `task db:integration:sqlite`，PostgreSQL 全量可运行 `task db:integration:postgres`。

## 成本与输出

用户提供的同机实测：后端检查约 2.8 秒（测试强制重跑 + vet），前端 lint 3.6 秒、build 10.5 秒、缓存命中的 `task check` 11.1 秒；Swagger 生成约 20.1 秒。实际 `task backend:check` 使用 `go test ./...`，允许测试缓存，并未设置 `-count=1`。

当时 48 个 SQLite 契约约 28.9 秒，48 个 PostgreSQL 契约约 294 秒；单 PostgreSQL 契约约 7 秒，单 SQLite 契约断言约 0.4 秒、包含 task/go 开销约 3.4 秒。数量会随新增测试变化，这些数据是成本参考，不是性能保证。

`db:check` 的三个依赖并行运行，总时长主要取决于 PostgreSQL。当前 `startPostgres` 每次启动独立容器，测试随后执行 migration；测试未调用 `t.Parallel()`，容器准备成本逐个累积。等待时间与 token 消耗分别优化：缩小验证范围减少时间，日志摘要减少上下文消耗。

需要全量时将输出保存到本次任务专用日志：

```sh
task db:check > /tmp/simple-inventory-db-check.log 2>&1
```

以工具返回的进程退出码判断成功，随后读取摘要；失败时检索对应 `FAIL` 和相关错误。持续运行时约每 30–60 秒读取一次状态并报告进度；完整输出保留在日志中。必要用例被跳过或无用例匹配时，应明确报告未验证。

## 后续测试基础设施优化

优先方案是每个集成测试进程只启动一个 PostgreSQL 容器，每个用例创建独立数据库并在清理时删除。这样仍隔离数据、序列、触发器和迁移版本，减少重复启动容器的成本。迁移用例继续从空库或指定旧版本开始；所有 helper 中硬编码的 `integration` 数据库名需改为当前用例数据库名。进程退出时清理容器，失败及单用例运行也需覆盖。

第二步再考虑迁移模板数据库：普通业务契约从已迁移且已关闭连接的模板克隆，迁移生命周期、环境种子、旧数据升级及备份恢复用例继续走各自真实准备流程。不能用模板代替 migration 验证，也不能让多个用例共享可修改的业务库。

容器复用稳定后才考虑有限并行；单加 Go 的 `-parallel` 不会让当前串行用例并发。上述方案尚未实施，收益需要对同一套契约重新实测，并验证测试隔离与容器清理。
