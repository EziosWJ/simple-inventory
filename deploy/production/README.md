# Linux 单机部署、整套备份与恢复

本包使用 PostgreSQL 17、内嵌 React 的 Go API 和 Caddy。运行时需要 Docker Engine / Compose、Python 3、Linux `cron`；不需要 Vite 或 Node。首次初始化、升级和恢复由维护者执行，日常录单使用两名独立经营者账号。正式云服务器、DNS 和真实证书尚需另行安排。

## 发布与配置

从仓库根目录执行 `deploy/production/build-images.sh <已验收版本号>`，得到同版本的 `simple-inventory-api` 和 `simple-inventory-migrate` 镜像。保留镜像 ID 和对应源码版本；升级前将旧镜像留在本机或可信镜像仓库，直到旧备份过了保留期。恢复工具按记录的镜像 ID 启动，不会用被重打标签的新镜像代替旧版。

复制 `.env.example` 为本目录私有 `.env`，执行 `chmod 600 deploy/production/.env`。设置发布版本、独立项目名、数据库随机密码（至少 20 字符）、JWT 随机密钥（至少 32 字符）。配置文件及备份不得提交到 Git。域名、浏览器 Origin 和可信代理必须匹配：`PUBLIC_ORIGIN` 包括非标准 HTTPS 端口；代理地址固定，后端和数据库无公开端口。改变子网时同步 `DB_IP`、`API_IP` 和 `PROXY_IP`，三个地址不得重复。

## 受控初始化

1. 保持 `PUBLIC_BIND=127.0.0.1`，临时设置 `DOMAIN=localhost`、`PUBLIC_ORIGIN=https://localhost`、`CADDYFILE` 为本包 `Caddyfile.test` 的绝对路径。本地 CA 只用于受控初始化/隔离验收，不安装到操作系统、不宣称为正式证书。远程维护使用 SSH 隧道访问回环端口。
2. 执行 `python3 deploy/production/manage.py migrate-start`。它先停止 API/代理写入者，再显式执行一次性 schema/seed 迁移；迁移或只读版本检查失败时不启动业务。`start` 只检查并启动，绝不隐式迁移。首次种子管理员为 `admin`，初始密码为 `admin123`，此时不要开放外网。
3. 用同一 `.env` 执行 `docker compose --env-file deploy/production/.env -f deploy/production/compose.yaml cp proxy:/data/caddy/pki/authorities/local/root.crt /tmp/inventory-local-ca.crt`。然后运行 `python3 deploy/production/initialize-accounts.py --url https://localhost --ca /tmp/inventory-local-ca.crt`，隐藏输入当前/新管理员密码及两名经营者的独立密码。脚本不写凭据文件，更换密码后重新登录，配置已有业务菜单角色；不会覆盖已有用户。中途中断时，以已完成步骤为准检查用户管理界面，勿重新创建同名账号。
4. 分别验证两名账号登录及共享单据。经营者是共同管理店铺的可信账号：现有菜单授权限制导航，部分既有用户/RBAC 管理接口仅要求登录，通知管理有管理员检查。此包保留原访问范围，不把菜单隐藏当作完整接口隔离。
5. 改回真实 `DOMAIN` 和对应 `PUBLIC_ORIGIN`，移除 `CADDYFILE` 临时覆盖、设置真实 `ACME_EMAIL`，初始化完成后才将 `PUBLIC_BIND` 改为 `0.0.0.0`。配置公网 DNS、80/443 和防火墙，再运行 `manage.py start` 重建应用/代理。Caddy 自动获取并续期公开证书，证书状态保存在 `proxy_data`；DNS、公网可达性与可写持久目录是准备条件。[Caddy 自动 HTTPS 文档](https://caddyserver.com/docs/automatic-https)

## 健康、持久性与故障

`/health` 为存活，`/ready` 为数据库就绪；Compose 等待数据库和 API 健康。生产关闭 `/swagger/**`。检查 `docker compose … ps` 和 `docker compose … logs --tail 100 api proxy db`，正常日志不显示密码/JWT。不要输出完整 `compose config` 或 `docker inspect` 环境到共享日志。代理覆盖客户端转发头，API 只信任配置的固定代理地址。[反向代理文档](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)

数据库、上传文件、代理状态均为命名卷。API 和迁移使用非 root 用户，镜像文件归属与上传挂载点已设置。`up --force-recreate` 或不带 `-v` 的 `down` 保留数据；禁止用 `down -v` 清理业务部署。只读迁移检查对缺表、缺版本和比当前应用更新的版本均拒绝启动，不会创建迁移表。

## 每日整套备份

开始前核对实际容器镜像与配置/迁移版本，不符时在停止业务前拒绝。务必先备份、后修改发布配置，避免混用当前业务库和另一版本镜像。

选择仅本部署专用的备份目录，权限 `700`。手动命令与 cron 使用同一流程：

```sh
python3 deploy/production/manage.py backup --directory /srv/inventory-backups
python3 deploy/production/schedule-backup.py --env /absolute/path/deploy/production/.env --directory /srv/inventory-backups
crontab -l
```

调度为**服务器本地时区每日 02:15**；安装时打印实际时区，维护者核对服务器 `date`/时区与业务维护窗口。安装者需有 Docker 访问权限，且 Linux cron 服务需运行。不会改动其他 cron 项；移除本项目任务使用相同命令加 `--remove`。查 `/srv/inventory-backups/backup.log`，失败退出非零；维护者每日检查最新 `BACKUP_COMPLETE`，发现失败当天重试。cron 本身不发送外部通知。

备份持有互斥锁，重叠触发明确拒绝。它暂停代理和 API 并等待进程退出，保持数据库在线，使用同一 PostgreSQL 主版本的 `pg_dump`；归档数据库、上传目录、实际配置/Caddyfile、发布镜像 ID、schema/seed 历史和 SHA-256。只有全部成功并核对后才原子公开带 `COMPLETE` 的目录；失败留下 `.partial-*` 供诊断，不视为备份，并尝试恢复原先运行的服务。恢复服务失败也返回非零，需要维护者检查健康状态。目录/文件默认 `700`/`600`，包含凭据，应放在受控且最好加密的磁盘。

成功后仅清理本目录中同项目、校验有效、超过 30 天的完整备份，保留本次最新成功备份；不明文件、损坏备份、半份产物和其他部署备份不删除。`python3 deploy/production/manage.py verify <完整备份目录>` 独立检查完整性。校验防意外损坏，不代替备份介质的访问控制。

每天将新完整目录通过受控 SSH/移动磁盘复制到另一设备，例如 `rsync -a <完整备份目录>/ <已配置的异地目录>/`。在另一设备复制本工具后运行 `verify`，核对成功标记；不得把真实凭据提交云盘公开链接或 Git。本包不绑定外部账号/存储，也不自动发送文件。每天频率意味着最多约一天的业务需要补录；只保存在同机时，主机/磁盘丢失仍可能同时丢失原数据和备份。

## 整套恢复与回退

先核对备份，准备其记录的**全部镜像 ID**（需事先保留或从可信仓库取回并核对）。恢复使用新项目、新目录、新网络/端口及全新数据卷，拒绝复用已有项目/目录：

```sh
python3 deploy/production/manage.py restore /srv/inventory-backups/<完整批次> \
  --target /srv/inventory-restore-<批次> --project inventory-restore-<批次> \
  --http-port 18081 --https-port 18444 --subnet 172.30.16.0/24 --proxy-ip 172.30.16.3
```

工具核对文件/manifest 和安全上传归档，导入同批数据库/文件，核对迁移历史，启动匹配旧镜像并只读检查版本。恢复端口仅绑定回环，先用隧道验收；不自动切换 DNS、不覆盖原业务。恢复目录中的 `deployment.env` 为必要私有配置，`images.override.yaml` 固定准确镜像；后续操作用 `docker compose --env-file <恢复目录>/deployment.env -f <恢复目录>/compose.yaml -f <恢复目录>/images.override.yaml …`。

必须实际核对两账号、原采购销售/明细/历史空值快照、库存/往来余额与流水累计、审计、文件读取和打印，再执行一笔代表性销售并核对出库/应收/审计。仅 `pg_restore --list` 或检查文件存在不足以证明可恢复。失败恢复项目保持隔离，调查前勿改原业务环境。

升级先停止业务操作，生成发布前整套备份并保留旧镜像，再更换 `.env` 发布版本，执行 `migrate-start`，核对 `/ready`、版本与核心业务。迁移失败不会启动新 API；不得用旧二进制直接读取已升级库。回退按上面的新项目恢复**发布前备份 + 匹配旧镜像**，验证后再由维护者切换入口。没有 migration down 承诺。恢复点之后的单据需从原环境核对并人工补录，尤其不得漏记真实已收付款；切换前停止写入，保留原环境用于追溯。
