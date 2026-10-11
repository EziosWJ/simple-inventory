# 简单进销存原生部署升级记录：销售明细查询与净销售合计（#66）

日期：2026-10-11（北京时间）。服务器：82.157.189.251。

## 结果

- 新版本：`native-20261011T120846Z`；上一版本 `native-20261010T021742Z` 保留在原目录。
- 源码：main 分支 `5ef7016`（merge: 增加销售明细查询与净销售合计 (#66)），构建时工作区干净；`BUILD-MANIFEST.json` 记录 git_head、git_ref、git_status 与构建参数。
- 构建：linux/amd64 静态 CGO（`CGO_ENABLED=1`、`netgo/osusergo/sqlite_omit_load_extension`、`-linkmode=external -extldflags=-static -s -w`），API 内嵌前端；三个程序 `file` 均显示 statically linked，`ldd` 显示 not a dynamic executable。
- 线上前端指纹与本地最终构建一致：`assets/index-HUzCxp7i.js`（sha256 c2ccd94ed26e5a70ec32e43a7585bc086ab66757fae415a2902efe5d8284cebd）、`assets/index-Bm9xICUl.css`（sha256 175d8ab2815a64d9de09b4fccee2014a587392aa3702d9a66df84efa54cee80e）。
- 数据库无新增迁移：停写期间 `migrate up --kind all` 输出 no migrations to run（schema 26、seed 24），切换前后迁移集合一致。
- 私有配置未改动：升级前核对“按原 Docker 配置重新生成 native-config”的结果与现网文件完全一致，`prepare` 未改变线上配置。
- 原版本目录、旧 Docker 镜像、Compose、`.env` 与旧 unit 全部保留，回退入口不变。

## 切换前备份

```text
/opt/simple-inventory/backups/native-before-native-20261011T120846Z-20261011T121405Z
```

包含数据库 custom dump（pg_restore --list 通过）、附件归档与属主、原生与 Docker 私有配置、systemd unit、compose.yaml、.env、上一版本 release 归档和迁移集合，校验通过并写入 COMPLETE。

## 检查结果

- `UPGRADE_OK native-20261011T120846Z from native-20261010T021742Z`，迁移集合前后一致。
- 本机 `/health`、`/ready` 与 `https://jxc.ezios.cn/ready` 均返回 200；服务 active 且 enabled；启动后日志没有 error/panic。
- 发布目录 `SHA256SUMS` 在服务器上校验通过。
- 内嵌前端包含本次新增的销售查询界面标记（“按明细”“全部匹配明细合计”“有效退货”“数量合计”）；已部署 API 二进制包含 `sales/items` 路由。
- 应用用户可读取全部历史附件并写入上传目录（1 个附件）。
- 手动执行每日备份 `backup-native.py` 成功（`native-daily-20261011T121516Z`），完成后 `/ready` 恢复 200；03:30 定时器配置未改动。
- 升级前在构建机执行 `task check` 通过；双库契约与真实浏览器销售查询验收见仓库 `docs/sales-query-acceptance.md`。

## 未执行项

- 未使用经营者账号手工登录和录单：销售明细视图、净销售合计、快捷条件与排序需要在真实账号下验收（本次只验证到健康检查、静态资源、附件与备份链路）。
- 未运行向正式库写入测试单据的业务验收。
- 未修改 1Panel、数据库容器、证书和反向代理配置；PostgreSQL 宿主机端口发布范围仍未调整。
- 未配置异机备份副本。

## 日常命令

```sh
sudo systemctl status simple-inventory.service
curl -fsS https://jxc.ezios.cn/ready
sudo python3 /opt/simple-inventory/native-tools/backup-native.py
sudo python3 /opt/simple-inventory/native-tools/upgrade-native.py <下一版本目录名>
```
