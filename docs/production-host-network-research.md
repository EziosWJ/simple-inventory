# 生产服务器 Docker 网络与开机启动调研

调研目标：核实目标服务器的 PostgreSQL 容器网络、主机资源和启动方式，并据此整理 Simple Inventory 部署方案。服务器信息来自 2026-10-03 的只读 SSH 查询；没有读取容器环境变量、数据库凭据或反向代理站点配置，也没有修改远端状态。后面的 Docker 行为说明引用 Docker 官方文档。

## 目标服务器只读检查结果

- 系统为 Ubuntu 22.04，2 vCPU，内存约 1.9 GiB（检查时可用约 1.2 GiB），根文件系统 50 GiB、可用约 31 GiB；`/opt` 当前已有 1Panel 数据目录，没有 Simple Inventory 目录。
- Docker Engine 26.1.2 正在运行，`docker.service` 和 `1panel` 服务均为 enabled/active。`ubuntu` 用户不在 Docker 组，但可使用 `sudo -n docker`；部署命令应通过 sudo 或 systemd 执行，不需要把普通账号加入 Docker 组。
- PostgreSQL 容器镜像标记为 `postgres:18.6-alpine`，容器名 `1Panel-postgresql-NPcN`，在 `1panel-network` 上的 DNS alias 为 `postgresql`，容器地址 `172.18.0.2`。网络为本机 bridge，子网 `172.18.0.0/16`、网关 `172.18.0.1`；该网络目前还连着 1Panel 私有镜像仓库。
- PostgreSQL 数据以宿主机目录 `/opt/1panel/apps/postgresql/postgresql/data` 持久化。宿主机没有运行独立 PostgreSQL systemd 服务。
- PostgreSQL 的 `5432` 端口通过 Docker 发布到 `0.0.0.0:5432` 和 `[::]:5432`。这表示绑定所有宿主机接口；本次确认 UFW 为 inactive，云厂商安全组规则未知，因此不能断言公网当前可达或不可达。应用容器在同一 Docker network 中可走 `postgresql:5432`，不需要经过此宿主机端口映射；部署本身不需要调整该现有映射。
- `1panel-network` 当前连着 PostgreSQL 和 1Panel 私有镜像仓库。加入该 network 后，Simple Inventory API 将能与这两个容器直接通信；部署前应确认接受此共享范围。
- 1Panel OpenResty 使用 host network，监听宿主机 `80/443`。它不在 `1panel-network` 上，不能直接用 Docker network DNS 访问应用容器；适合让 API 将端口绑定在宿主机 `127.0.0.1`，再在 1Panel 配置反向代理上游。

因此，主机集成 Compose 草案已改为加入 `external: true` 的 `1panel-network`，不自行创建应用网段或新增 PostgreSQL 服务。API 数据库地址使用 `postgresql:5432`。反向代理链路使用 `127.0.0.1:<本机空闲端口>`；已确认候选 `18080` 当前未监听，启动后仍需核对 API 观察到的可信代理地址。

建议在 `/opt/simple-inventory` 下新建项目目录，镜像由 Docker 管理；Compose 文件与 `.env`、生产配置、上传文件分别放入部署、私密配置和数据目录。应用镜像固定版本，API 与 migration 共用同一个镜像；数据库用户应为 Simple Inventory 单独创建，不复用 PostgreSQL 管理员账号。目标机只有 2 vCPU、约 1.9 GiB 内存；不建议在该机现场构建 Node/Go 镜像。本地 WSL 的 Docker CLI 当前无法连接 Docker Desktop，因此发布镜像还需由 CI 或另一台已配置 Docker 的构建机生成、归档后传输。

当前项目最终技术验收记录核对过 PostgreSQL 17.10，而服务器运行的是 18.6。迁移尚未执行前，应使用单独测试数据库验证当前发布对 PostgreSQL 18 的空库 migration、启动检查、核心采购/销售事务和备份恢复兼容性；不要先在 1Panel 现有业务库上试跑迁移。

启动策略建议让 systemd 以前台 `docker compose up` 方式作为应用唯一重启管理者，Compose API 服务设置 `restart: "no"`；这样服务异常退出由 systemd 观察并重启，主机启动由 enabled unit 拉起。Docker 服务本身已 enabled。不要同时让 systemd 与 Docker restart policy 重复管理同一个应用容器。[Docker 自动启动容器](https://docs.docker.com/engine/containers/start-containers-automatically/)

## 已有 Docker network 的接入方式

Compose 可以把服务加入在 Compose 项目之外创建的网络：在顶层 `networks` 中将逻辑网络标为 `external: true`，并通过 `name` 指定 Docker 上的实际网络名；再在 API 服务的 `networks` 中引用该逻辑网络。Compose 不会创建或管理 external network，运行 `docker compose up` 前该网络必须已经存在，否则启动失败。[Docker Compose 网络文档](https://docs.docker.com/compose/how-tos/networking/)；[Compose networks 字段参考](https://docs.docker.com/reference/compose-file/networks/)

```yaml
services:
  api:
    networks:
      - shared

networks:
  shared:
    name: ${SHARED_DOCKER_NETWORK}
    external: true
```

若 PostgreSQL 与 API 同在同一自定义 bridge network，容器可通过网络 DNS 使用稳定的容器名、Compose 服务名或配置的网络 alias 访问对方；使用数据库容器的内部监听端口（PostgreSQL 通常为 `5432`），不需要为同网络容器间通信依赖宿主机发布端口。容器 IP 可在重建后变化，应使用名字而不是固定 IP。[Compose 网络和服务发现](https://docs.docker.com/compose/how-tos/networking/)；[Bridge 网络驱动](https://docs.docker.com/engine/network/drivers/bridge/)

建议在部署前于服务器只读核实：`docker network ls`、`docker network inspect <网络名>`、`docker inspect <PostgreSQL容器>`。确认它是 API 可加入的本地网络、network driver/scope、PostgreSQL 容器在该网络上的 DNS 名称/alias，以及其数据库端口。自定义 bridge 支持 DNS 名称发现；Docker 默认 `bridge` 网络不提供同等的按容器名 DNS 能力，而且 Docker 文档不建议在生产场景使用默认 bridge。[Bridge 网络驱动](https://docs.docker.com/engine/network/drivers/bridge/)

加入共享网络意味着同网容器之间端口可直接互通。Docker 文档也建议按服务隔离网络；因此应只把确实需要访问 PostgreSQL（以及确实需要被反向代理发现）的应用服务接入共享网络，并确认共享网络成员可信。若反向代理和数据库处于不同网络，可用双网络连接 API，使数据库继续留在它自己的网络。[Bridge 网络驱动](https://docs.docker.com/engine/network/drivers/bridge/)；[Compose 混合网络示例](https://docs.docker.com/compose/how-tos/networking/)

如果 PostgreSQL 不是同一 Docker daemon 上的容器，而是在另一台主机/托管服务，当前主机的本地 bridge network 不会自动跨 Docker 主机延伸。应连接可路由的数据库主机地址并按需配置防火墙；Docker 将跨主机容器互联列为 overlay 或操作系统路由等问题。[Docker 网络概览](https://docs.docker.com/engine/network/drivers/bridge/)；[Docker 网络驱动](https://docs.docker.com/engine/network/drivers/)

如果 PostgreSQL 实际运行在 Docker 宿主机，而非容器中，容器里的 `127.0.0.1` 指向容器自身。Linux 上可通过 Compose `extra_hosts` 将 `host.docker.internal` 映射到 `host-gateway`，但数据库还必须监听容器能够到达的宿主机接口，并允许相应来源连接；是否开放宿主机数据库端口应由主机防火墙控制。[Compose `extra_hosts` 示例](https://docs.docker.com/compose/how-tos/networking/)；[Docker host-gateway 映射](https://docs.docker.com/compose/how-tos/networking/)

## 容器与服务器开机启动

Docker Engine 官方说明 Debian 和 Ubuntu 上 Docker 服务默认随系统开机启动；Docker restart policy 控制容器在退出或 Docker daemon 重启后的行为。Compose 的 `restart: unless-stopped` 可让容器异常退出后重启、daemon 重启后恢复，但如果运维人员手动停止了容器，它不会在 daemon 重启时自动重新启动。它是容器级策略，不负责执行数据库迁移或更新 Compose 配置。[Docker Linux 安装后配置](https://docs.docker.com/engine/install/linux-postinstall/)；[Docker 自动启动容器](https://docs.docker.com/engine/containers/start-containers-automatically/)；[Compose `restart` 字段](https://docs.docker.com/reference/compose-file/services/)

Docker 官方明确提醒不要把 Docker restart policy 与宿主机层的 process manager（例如 systemd）同时用于管理同一容器的重启，因为两套管理器可能冲突。若选择 Docker 自己管理容器恢复，应确认 `docker.service` 已启用，并为 API 配置适当的 Compose `restart` 策略；一次性 migration 服务应使用 `restart: "no"`，避免成功或失败退出后循环重跑。若选择 systemd 直接管理容器/Compose 生命周期，则应避免再为同一 API 容器配置 Docker restart policy，并明确服务失败、停止和启动时的 Compose 命令行为。[Docker 自动启动容器](https://docs.docker.com/engine/containers/start-containers-automatically/)；[Compose `restart` 字段](https://docs.docker.com/reference/compose-file/services/)

对本服务器的建议：Docker daemon 已 enabled/active，因此主机开机时 Docker 会先启动。应用生命周期由 systemd unit 以前台 Compose 进程单独管理，API 的 Docker restart policy 设为 `no`；Compose 迁移任务也是一次性命令。发布操作显式运行迁移，systemd 只启动 API，不在重启中隐式升级数据库。此选择遵守 Docker 关于不让两套 process manager 同时负责同一容器重启的建议。

## 服务器核实与连接验收建议

执行变更前先确认网络真实状态，不预设网络叫 `postgres`、`proxy` 或其他名称。检查项：

1. 已核实 `1panel-network` 为本地自定义 bridge，PostgreSQL 的 alias 是 `postgresql`，数据库容器端口为 `5432`。
2. 部署前确认目标 Compose external network 仍存在、API 端口可用、1Panel OpenResty 上游配置目标域名，并确认 PostgreSQL 中专用业务角色/数据库是否已创建。
3. 启动 API 前从共享网络验证 DNS 解析和 TCP 连通，再用专用业务凭据运行 migration/check；TCP 可达不等于认证或 migration 成功。
4. 在隔离数据库完成 PostgreSQL 18 兼容性验证后，备份并创建/确认业务数据库，再显式 migration，最后启动 API。
5. 通过 1Panel 配置 HTTPS 反向代理，验收 `/health`、`/ready`、前端深链、上传和登录；确认公网只开放必要的 Web 入口。

网络接入配置的关键值（external network name、数据库网络 alias 和端口）已按只读检查结果记录；部署期间仍应复查 1Panel 变更后的实时状态。
