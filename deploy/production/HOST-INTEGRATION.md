# 接入现有 PostgreSQL 与反向代理

此部署方式使用单一应用镜像，包含内嵌 React 的 API、迁移工具、备份工具、基础配置和 PostgreSQL migration。生产配置和密钥留在服务器，通过只读挂载提供给容器。API 仅发布到主机回环端口，适合接入主机上已有的 Nginx/Caddy。目标服务器已有 PostgreSQL 容器在 `1panel-network`，API 会加入此 external network 并通过容器 alias `postgresql:5432` 访问数据库。原 `compose.yaml` 的内置 PostgreSQL、Caddy 和整套备份/恢复流程仍可独立使用。

## 准备发布文件

在仓库根目录构建并检查同一个镜像的服务别名拥有相同 image ID：

```sh
task container:build -- phase5-final-20261002
docker image inspect simple-inventory:phase5-final-20261002 simple-inventory-api:phase5-final-20261002 simple-inventory-migrate:phase5-final-20261002 --format '{{.Id}}'
docker save simple-inventory:phase5-final-20261002 | gzip > simple-inventory-phase5-final-20261002.tar.gz
```

在服务器创建如下目录，将镜像归档、`deploy/production/compose.host.yaml`（复制为 `compose.yaml`）、`deploy/production/simple-inventory.service`、`server/configs/config.prod.example.yaml` 及 `deploy/production/.env.host.example` 复制到对应位置：

```text
/opt/simple-inventory/
├── .env
├── compose.yaml
├── simple-inventory.service
├── config/config.prod.yaml
├── data/uploads/
└── releases/<版本>/simple-inventory.tar.gz
```

systemd unit 安装在 `/etc/systemd/system/simple-inventory.service`。加载镜像：

```sh
sudo docker load -i /opt/simple-inventory/releases/phase5-final-20261002/simple-inventory.tar.gz
```

准备私有配置、上传目录和发布归档目录：

```sh
sudo install -d -m 700 /opt/simple-inventory/config
sudo install -d -m 755 /opt/simple-inventory/data
sudo install -d -o 100 -g 101 -m 750 /opt/simple-inventory/data/uploads
sudo install -d -m 700 /opt/simple-inventory/releases/phase5-final-20261002
sudo install -m 600 server/configs/config.prod.example.yaml /opt/simple-inventory/config/config.prod.yaml
sudo install -m 600 deploy/production/.env.host.example /opt/simple-inventory/.env
sudo install -m 644 deploy/production/simple-inventory.service /opt/simple-inventory/simple-inventory.service
```

编辑 `/opt/simple-inventory/config/config.prod.yaml`，设置数据库连接到 `postgresql:5432`，使用专用数据库和最小权限业务用户，配置随机 JWT 密钥及准确的浏览器 Origin。项目与 PostgreSQL 同在 `1panel-network`，不要写宿主机映射端口 `5432`、容器 IP 或 `127.0.0.1`。

编辑 `/opt/simple-inventory/.env`，至少设置 `INVENTORY_RELEASE`、`CONFIG_FILE=/opt/simple-inventory/config/config.prod.yaml`、`UPLOADS_DIR=/opt/simple-inventory/data/uploads`、`APP_DOCKER_NETWORK=1panel-network`、`TRUSTED_PROXY_IP=172.18.0.1` 和 `API_PORT=18080`。把 `INVENTORY_IMAGE_PREFIX` 设为实际镜像仓库前缀。`.env` 和 `config.prod.yaml` 均设为 `600`。Compose 会把 API 加入现有 external network，不会创建新的业务 network。

目标服务器使用已有 PostgreSQL，不启动第二个数据库容器。

## 初始化和启动

先运行一次性 migration。数据库必须已创建专用数据库和业务用户，且容器能通过 `1panel-network` 解析并访问 `postgresql:5432`：

```sh
cd /opt/simple-inventory
sudo docker compose --env-file .env -f compose.yaml run --rm migrate up --kind all
sudo docker compose --env-file .env -f compose.yaml run --rm migrate check --kind all
```

将 systemd unit 安装到系统目录并启用：

```sh
sudo install -m 644 /opt/simple-inventory/simple-inventory.service /etc/systemd/system/simple-inventory.service
sudo systemctl daemon-reload
sudo systemctl enable --now simple-inventory.service
sudo systemctl status simple-inventory.service
```

systemd 前台运行 Compose 并作为应用的唯一重启管理者；API 容器不启用 Docker restart policy。Docker 服务本身已经 enabled/active，会先于应用 unit 启动。API 启动时只读检查 migration 版本；数据库未完成初始化或版本不匹配时，API 不会就绪。生产 Swagger 保持关闭。

在 1Panel/OpenResty 配置 HTTPS 和域名，将上游设为 `http://127.0.0.1:18080`（或 `.env` 中配置的 `API_PORT`）。转发 `Host`、`X-Forwarded-For` 和 `X-Forwarded-Proto`；应用先配置 Docker 网关 `172.18.0.1` 为可信代理，启动后再按实际 peer address 核验。外部防火墙不应开放 API 端口。`/health` 是存活检查，`/ready` 是数据库就绪检查。

升级时先备份数据库和上传目录、保留旧镜像，再停止 systemd 服务，载入新版本镜像，显式运行 migration 和 check，最后启动 systemd 服务。回退需要恢复与旧镜像匹配的数据库备份，不能让旧版本直接读取已升级的 schema。

## 备份边界

`manage.py backup` 和 `manage.py restore` 当前针对原 `compose.yaml` 管理的 PostgreSQL/Caddy 部署：它们会暂停代理和 API，备份 Compose 内的 PostgreSQL、上传卷、配置及准确镜像。此主机集成模式使用 1Panel 外部 PostgreSQL 和绑定挂载目录，需要单独制定 PostgreSQL 18 备份与恢复方式，并将 `/opt/simple-inventory/data/uploads`、私有配置和镜像版本一同纳入恢复演练。不要把只备份数据库或只备份上传目录视为整套恢复。
