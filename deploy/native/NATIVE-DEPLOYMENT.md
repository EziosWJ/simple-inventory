# 简单进销存：Ubuntu 直接部署方案

日期：2026-10-10。服务器：ubuntu@82.157.189.251。

## 目标与分工

构建并发布当前工作区代码（包括用户已有的未提交改动），前端内嵌于 Go API。应用由 systemd 直接运行；PostgreSQL 和 OpenResty 继续由 1Panel 管理。代理继续使用 127.0.0.1:18080。此次不修改 1Panel、数据库容器、证书及反向代理配置。

部署人员负责构建、发布、应用用户、私有配置、备份、迁移检查、切换、回退准备和验证。用户负责 1Panel 中的数据库端口与安全组设置。

## 已核实的现状

- Ubuntu 22.04 / amd64 / glibc 2.35；内存 1.9 GiB，磁盘剩余约 29 GiB。
- 原应用镜像 simple-inventory:http-uuid-20261008-1，systemd 启动 Docker Compose。
- jxc.ezios.cn 经 OpenResty host 网络代理至 127.0.0.1:18080；HTTPS /ready 返回 200。
- PostgreSQL 容器 1Panel-postgresql-NPcN，18.6；数据库和业务用户均为 jxc。
- 上传目录 /opt/simple-inventory/data/uploads；原容器 UID:GID=100:101。
- 原 migration 最新记录 schema=26、seed=24。切换前将用新工具核验完整迁移集合。

## 目标结构

```text
https://jxc.ezios.cn → 现有 OpenResty
                        ↓ 127.0.0.1:18080
                  Go API + 内嵌 React（systemd）
                        ↓ 127.0.0.1:5432
                  现有 PostgreSQL 容器

/opt/simple-inventory/
├── NATIVE-DEPLOYMENT.md             本方案
├── NATIVE-DEPLOYMENT-RESULT.md      实施结果
├── current -> releases/<版本>
├── native-config/config.prod.yaml  私有配置
├── data/uploads/                   原附件目录
├── releases/<版本>/
│   ├── simple-inventory-api
│   ├── simple-inventory-migrate
│   ├── simple-inventory-backup     SQLite 专用；不用于 PostgreSQL 备份
│   ├── configs/config.yaml
│   ├── configs/config.prod.yaml -> /opt/simple-inventory/native-config/config.prod.yaml
│   ├── migrations/
│   ├── BUILD-MANIFEST.json
│   └── SHA256SUMS
├── native-tools/                   部署、备份和回退工具
└── backups/native-*/               切换前或日常完整备份
```

## 构建与配置

先运行 task check 与相关前端回归；使用 task build 生成内嵌前端的发布程序。构建机 glibc 比目标机新，因此发布程序使用 CGO_ENABLED=1、netgo/osusergo、sqlite_omit_load_extension 和外部静态链接重新构建，保留 SQLite 迁移及备份能力（不加载动态扩展），并检查 ELF 无动态解释器。服务器无需安装 Go、Node.js 或 musl。

保留原 JWT 密钥、业务账号、数据库密码、CORS 等配置。复制原私有配置到 native-config，由 root 和专用 simple-inventory 用户组访问（目录 750、文件 640）；原 Docker 配置与 .env 留存。只调整数据库 URL 的主机为 127.0.0.1、HTTP 监听地址为 127.0.0.1:18080、可信代理为 127.0.0.1、上传路径为现有宿主机绝对路径，禁用 Swagger。

专用 simple-inventory 系统用户运行应用。上传目录及文件改由该用户所有，保留原权限，并记录原数值 UID/GID，以供 Docker 回退。systemd 使用 Restart=on-failure、优雅停止、NoNewPrivileges 和仅上传目录可写的文件系统保护；启动前只执行迁移 check，不自动升级数据库。

## 实施顺序

1. 构建并记录 Git HEAD、工作区差异及发布文件 SHA256；上传到新版本目录。
2. 用新迁移工具连接现有数据库执行 check；核对 ELF 在服务器运行正常。
3. 临时服务仅监听 127.0.0.1:18081，验证 /health、/ready、页面及静态文件。迁移状态不匹配则先暂停切换并调查，不在预检阶段更新正式数据库。
4. 停止临时服务和旧应用，进入短暂停写窗口。数据库与附件一起备份，并保存旧 systemd unit、Compose、.env、私有配置、镜像 ID、附件属主元数据。验证 pg_restore --list、附件归档与校验和，成功后写 COMPLETE 标记。
5. 停写期间显式执行 migrate up --kind all（预期无新增迁移），再执行 check。记录前后版本。
6. 切换 current，替换应用 systemd unit，启动并检查本机 /ready、HTTPS /ready、页面与静态文件。
7. 验证应用用户能读取历史附件并写入上传目录；保留原镜像和旧配置。
8. 安装每日 03:30 的完整备份 timer。备份暂停应用以保证数据库与附件一致，完成或失败后恢复应用；仅验证成功的备份具有 COMPLETE 标记。不自动删除历史备份。

## 运维

```sh
sudo systemctl status simple-inventory.service
sudo journalctl -u simple-inventory.service -n 100 --no-pager
curl -fsS http://127.0.0.1:18080/ready
sudo python3 /opt/simple-inventory/native-tools/backup-native.py
sudo systemctl list-timers simple-inventory-backup.timer
```

应用异常退出由 systemd 重启；数据库短暂重启导致启动失败时会重试。/ready 负责确认数据库就绪；systemd 的进程存活不代替就绪监控。

后续升级：先检查和构建新版本，停写并完整备份，显式迁移，核验后切换 current 并重启。每个版本均保留构建清单及迁移文件。默认备份在同一台服务器，仍需把完整备份加密复制到另一台机器或对象存储；本次不擅自配置外部账号。

## 回退

本次先确认新旧 schema/seed 集合一致。如果迁移过程中发生实际数据库升级，不自动启用旧应用，应保持停写并基于完整备份安排恢复。

没有 schema 变化时，部署失败会恢复原 systemd unit 和附件属主，再启动旧 Compose。上线后手动回退入口为 native-tools/rollback-docker.py，脚本先用旧镜像执行只读迁移检查；不匹配则拒绝回退。回退不覆盖或删除已有业务数据。恢复数据库备份会丢失备份后的新增数据，不能直接执行。

## 用户在 1Panel 中操作

- 现有代理 127.0.0.1:18080 和 HTTPS 证书无需调整；应用切换成功后核对站点可以登录和查询。
- PostgreSQL 当前发布到 0.0.0.0:5432 和 IPv6 全地址。核对所有使用者后，将发布限制到 127.0.0.1:5432（容器内端口仍为 5432，保留现有 Docker 网络供其他容器连接），或在云安全组限制来源。不要删除重建数据库或改变数据卷。
- 调整数据库容器可能造成短暂不可用，完成后检查 jxc.ezios.cn/ready。应用依赖该端口，请勿直接取消宿主机端口发布。
- 此次没有检查云安全组；已安排隔离数据库与附件恢复验证。实施结果文件会记录实际已完成验证与未执行项。

## 验收边界

自动验证构建、迁移一致性、健康/就绪、HTTPS、前端资源和附件文件权限。前端业务回归使用隔离或拦截 API 的夹具，不能代替经营者在正式账号下验收。不会向正式数据库写入测试单据。

## 后续升级

2026-10-11 增补：`native-tools/upgrade-native.py` 把下面这段手工顺序固化为带锁、带备份和带回退的一步操作，在 `deploy-native.py prepare` 之后使用。

```sh
cd /opt/simple-inventory
sudo python3 native-tools/deploy-native.py prepare <新版本目录名>   # 校验和、只读迁移检查、18081 预检
sudo python3 native-tools/upgrade-native.py <新版本目录名>         # 备份、显式迁移、切换 current、启动并等待就绪
```

顺序与保障：

- 取 `backups/.native.lock` 互斥锁，校验发布目录 `SHA256SUMS`，确认新版本已完成 prepare（`configs/config.prod.yaml` 为符号链接）且与当前版本不同。
- 只读 `migrate check` 并记录迁移集合，然后停止 `simple-inventory.service`，写一份完整备份（数据库 custom dump、附件、私有配置、unit、当前 release 归档），备份成功后才继续。
- 显式执行 `migrate up --kind all`，随后核对迁移集合：发生变化就保持停止并报错，不让旧程序读取已升级 schema。
- 再执行一次 `migrate check`，把 `current` 指向新版本目录，`daemon-reload` 并启动，等待 `127.0.0.1:18080/ready` 与 `https://jxc.ezios.cn/ready` 通过，最后再次核对迁移集合未变。
- 失败时（迁移集合未变）恢复原 `current` 指向、原 unit 和附件属主，并启动旧版本；旧版本目录从不删除。结果写入 `native-tools/last-upgrade.txt`。
- 升级前在构建机执行 `task check`，并用 `CGO_ENABLED=1 GOOS=linux GOARCH=amd64` 与 `netgo`、`osusergo`、`sqlite_omit_load_extension`、`-linkmode=external -extldflags=-static -s -w` 构建静态程序；发布目录内容与首次部署相同（含 `BUILD-MANIFEST.json`、`SHA256SUMS`、`source-workspace.tar.gz`）。
