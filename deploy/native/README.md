# 原生部署（Ubuntu 直接部署）

本目录保存服务器 `ubuntu@82.157.189.251` 上 `/opt/simple-inventory` 实际运行的原生部署工具、systemd unit 与运维文档。**服务器上的文件是运行时权威副本**，本目录用于追溯（2026-10-11 从服务器同步，工具内容逐字保留）。以后改名、加参数或修脚本时，应先改仓库再同步到服务器。

## 部署形态

```text
https://jxc.ezios.cn → 现有 OpenResty（1Panel 管理）
                        ↓ 127.0.0.1:18080
                  Go API + 内嵌 React（systemd: simple-inventory.service）
                        ↓ 127.0.0.1:5432
                  现有 PostgreSQL 18 容器（1Panel 管理，库与业务用户均为 jxc）
```

`/opt/simple-inventory/` 的目录结构、发布目录内容、静态构建参数、私有配置与备份/回退约定见 [NATIVE-DEPLOYMENT.md](NATIVE-DEPLOYMENT.md)。仓库里另有两种打包方式：`deploy/production/`（单镜像 Compose，自带 PostgreSQL 与 Caddy）与 `deploy/production/HOST-INTEGRATION.md`（单镜像接入已有 PostgreSQL 与主机代理）；本目录对应 2026-10-10 切换后的原生方式，PostgreSQL、OpenResty 与证书仍由 1Panel 管理。

## 仓库与服务器对应关系

| 仓库 | 服务器 |
| --- | --- |
| `tools/native_common.py` | `/opt/simple-inventory/native-tools/native_common.py` |
| `tools/deploy-native.py` | 同目录；`prepare` 预检与首次部署 `switch` |
| `tools/upgrade-native.py` | 同目录；升级切换：备份、显式迁移、失败回退 |
| `tools/backup-native.py` | 同目录；每日完整备份（暂停应用保证库与附件一致） |
| `tools/rollback-docker.py` | 同目录；回退到切换前的 Docker 应用 |
| `tools/verify-native-restore.py` | 同目录；校验备份恢复结果 |
| `systemd/simple-inventory.service` | `/etc/systemd/system/simple-inventory.service`（与 `deploy-native.py` 写入的内容一致） |
| `systemd/simple-inventory-backup.service`、`.timer` | `/etc/systemd/system/`，服务器时区每日 03:30 |
| [../docs/native-deployment-result-20261010.md](../docs/native-deployment-result-20261010.md) | `/opt/simple-inventory/NATIVE-DEPLOYMENT-RESULT.md` |
| [../docs/native-upgrade-20261011.md](../docs/native-upgrade-20261011.md) | `/opt/simple-inventory/NATIVE-UPGRADE-20261011.md` |

切换前保留的旧 Docker 方案 unit 与仓库 `deploy/production/simple-inventory.service` 完全一致，未重复保存。运行状态文件（`native-tools/initial-backup.txt`、`native-tools/last-upgrade.txt`、`backups/.native.lock`）只存在于服务器，不入库。

## 日常入口

```sh
sudo systemctl status simple-inventory.service
sudo journalctl -u simple-inventory.service -n 100 --no-pager
curl -fsS https://jxc.ezios.cn/ready
sudo python3 /opt/simple-inventory/native-tools/backup-native.py
sudo systemctl list-timers simple-inventory-backup.timer
```

升级顺序：构建机执行 `task check` → 按 NATIVE-DEPLOYMENT.md 的参数构建静态程序并组装发布目录 → 上传到 `releases/<新版本>` → `deploy-native.py prepare <新版本>`（校验和、只读迁移检查、18081 预检）→ `upgrade-native.py <新版本>`（加锁、完整备份、显式迁移、切换 `current`、就绪检查，失败恢复旧版本）。本次升级没有 schema 变化时，也可直接把 `current` 指回上一版本目录后重启实现回退；要回到切换前的 Docker 应用用 `rollback-docker.py`。

## 同步核对

2026-10-11 同步，逐文件 SHA256（在仓库根目录用 `(cd deploy/native && sha256sum -c <<'SUMS' … SUMS)` 或在服务器上比对）：

```text
d6cd0576a44ae56e4cabd226f91e023118ad67d5d580d712243d5cf76f11bfeb  NATIVE-DEPLOYMENT.md
0d972ece36ec9abd1c38afaf72ba57265d11dd9dc8073d05c6fa38f34c945789  systemd/simple-inventory-backup.service
e10bbeca8e7dcb50fc0b0e3f6591565753de7db5505e037a1d4a37b28739c084  systemd/simple-inventory-backup.timer
2dd43c4de6c02087f040826071847f1a364c73fb178b56b25e5bad8e9d583750  systemd/simple-inventory.service
7d78231ef5fd647ec366c0d8c39437859dc064b782e2a8402317bc06ddc7c292  tools/backup-native.py
ac7632076e66ea272f612337221bfe01a8be78458797f6ed1e33d18f384059f2  tools/deploy-native.py
b68a167fc920dac1e162e92607c99325610aa5b32aa75e71094e4e0e6e521120  tools/native_common.py
dd6c7974f174042ef191da640f02a97a8f63061f0babccf5ab1263f2e248c15f  tools/rollback-docker.py
954c8e053acbb5501e5ebade70f390aec4951b31e5fbf2ddf7f18fe500f5062d  tools/upgrade-native.py
75dffa707e860ccfd46a097e1c72d9641e5fb2b4220f899c8c1d7bd715f0961b  tools/verify-native-restore.py
```

最近一次发布：`native-20261011T120846Z`（源码 main `5ef7016`，销售明细查询与净销售合计 #66），上一版本 `native-20261010T021742Z` 目录保留。
