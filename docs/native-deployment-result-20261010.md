# 简单进销存直接部署实施结果

实施日期：2026-10-10（北京时间）。服务器：82.157.189.251。

## 已上线

- 发布版本：native-20261010T021742Z。
- 源码：构建时的当前工作区，包含原有未提交改动；不是从旧镜像提取程序。
- 入口：https://jxc.ezios.cn。
- 原生进程：/opt/simple-inventory/current/simple-inventory-api，用户 simple-inventory。
- systemd：simple-inventory.service，已启用开机启动和失败重试；APP_ENV=prod、GIN_MODE=release。
- 监听：127.0.0.1:18080，现有 OpenResty 代理保持该上游。
- PostgreSQL 和 OpenResty 继续由 1Panel 管理，未修改其配置或重启其容器。
- 当前数据库连接使用 127.0.0.1:5432；数据仍是原 jxc 数据库和用户。
- 原上传目录沿用，改由专用应用用户所有；原数值属主记录已备份。
- 原 JWT、账号、密码和业务数据保留；Swagger 禁用。私有配置在 native-config，目录 750、文件 640。
- 原应用容器已停止，旧镜像、Compose、.env、Docker 私有配置和旧 systemd unit 仍保留以供回退。

## 发布记录

新版本目录中保留 BUILD-MANIFEST.json、SHA256SUMS、source-workspace.tar.gz 和双库 migration。API、迁移和 SQLite 备份工具均为 linux/amd64 静态 CGO 构建，无 ELF 动态解释器；通过 netgo、osusergo、sqlite_omit_load_extension 标签避免宿主机 glibc 版本依赖及 SQLite 动态扩展加载。服务器无需安装 Node.js 或 Go。

线上前端指纹已与本地最终构建核对：

```text
assets/index-CXy5h0Ug.js
SHA256 fef08302a867e77d5141ab3b3840023d7abeb5a90d5d6802091215212f4b0746
assets/index-Gjbpph_f.css
SHA256 f803bb776c638a3e01d59377c5fe5500685cb41f67ac40507114c5595bf72cf7
```

## 字典显示问题

只读核对线上字典：现有字典类型启用、未删除，41 个条目没有发现缺失。实际库存流水原因存在 PURCHASE（6 条）、PURCHASE_CANCEL（1 条）、SALE（6 条）、SALE_CANCEL（1 条）。

根因是库存流水页面把采购、销售的业务原因用库存调整原因字典显示，而后者只定义期初、盘盈、盘亏、报损、其他。因此错误显示“原值（字典项缺失）”。修复前的隔离回归明确复现 PURCHASE（字典项缺失）。

已修正库存流水原因类型与标签，覆盖采购入库、销售出库、采购退货、销售退货及对应取消，库存调整仍使用管理员配置的标签。真实页面和最终构建资源回归均通过。本次没有向线上字典插入数据，也没有改变业务记录或数据库结构。

## 发布前额外修正

- 送货单长单号在打印区域宽 360px，而可用宽仅 334px。单号字号从 12px 调为 11px，保留完整单行显示。
- 业务 UI 测试修正详情场景的关联列表和取消状态向后续用例泄漏的问题，并按字段标签定位整单备注。
- PDF 测试兼容中文与拉丁字体产生独立文字片段，同时继续核对完整单号与标签同一行。

## 备份与恢复验证

切换前完整备份：

```text
/opt/simple-inventory/backups/native-before-native-20261010T021742Z-20261010T021908Z
```

包含数据库 custom dump、附件、原配置、Compose、.env、原 systemd unit、迁移记录、镜像标识、原附件属主和校验清单；已通过验证并存在 COMPLETE 标记。

数据库迁移 schema=26、seed=24，全部迁移记录与新版本一致。停写期间显式运行 up，无新增迁移；切换前后记录一致。

已将切换前 dump 恢复到独立临时数据库，用新迁移工具连接该库并核对全部迁移记录。附件恢复到临时目录并逐文件比较 SHA256（1 个附件），通过后清理临时数据库和目录，未覆盖正式数据。

每日备份由 simple-inventory-backup.timer 于服务器时间 03:30（北京时间）触发；已启用并手动实测。实测备份目录：

```text
/opt/simple-inventory/backups/native-daily-20261010T021950Z
```

备份会暂停 API，保证数据库和附件一致；此次实测暂停约 6 秒，完成后应用正常恢复。失败也会尝试恢复 API。仅完整且校验通过的备份写入 COMPLETE 标记。不自动清理历史备份；需关注磁盘容量。

本次备份均存放在同一服务器，尚未配置异机或对象存储副本。

## 检查结果

- task check：通过（Go tests/vet、前端 lint/build）。
- task build：通过；之后针对最终前端重新生成嵌入资源并静态构建。
- 最终前端 lint/build、git diff --check：通过。
- 发布标签下 webui 和 database 测试：通过。
- 静态二进制 SQLite 空库迁移、migration check、备份完整性检查：通过。
- 时区、零单价、HTTP 保存、附件回归：通过；测试使用隔离 API。
- 完整业务 UI 回归：通过（23 个路由场景、两种桌面宽度、表单和弹窗离开保护）。
- 完整打印回归：通过；生成并核验真实 A4 PDF 的物理页数、页码、页面边界、末页内容、金额显隐和完整单号。
- 库存原因真实页面回归和最终构建资源回归：通过。
- 正式库 migration check、迁移前后记录对比、隔离恢复验证：通过。
- 本机和 HTTPS /ready：200；未认证采购接口：401。
- 正式端口由原生 Go 进程监听，原应用容器不再运行。

未执行 task db:check（本次没有数据库结构、迁移或后端业务逻辑变更）；未运行向正式库写入测试单据的业务验收；未使用正式账号完成手工登录和录单；未检查或调整云安全组。没有修改 REST API，未运行 api:docs。

## 日常命令与回退

```sh
sudo systemctl status simple-inventory.service
sudo journalctl -u simple-inventory.service -n 100 --no-pager
sudo systemctl list-timers simple-inventory-backup.timer
sudo python3 /opt/simple-inventory/native-tools/backup-native.py
curl -fsS https://jxc.ezios.cn/ready
```

需要回退原 Docker 应用时：

```sh
sudo python3 /opt/simple-inventory/native-tools/rollback-docker.py
```

该脚本先用旧镜像核对当前数据库迁移集合。不匹配则拒绝继续；匹配后禁用每日原生备份 timer，停止原生服务、恢复附件属主与旧 unit、启动旧 Compose。回退不会覆盖现有业务数据。此回退准备已静态核对，但未在成功上线后实际回切旧版本。

## 用户在 1Panel 中操作

1. jxc.ezios.cn 代理继续使用 http://127.0.0.1:18080，证书和上游无需调整。
2. PostgreSQL 发布范围目前仍为 0.0.0.0:5432 和 IPv6 全地址。核对其他服务连接方式后，在 1Panel 中限制宿主机端口到 127.0.0.1:5432，保留容器内 5432 和现有网络；或配置云安全组仅允许必要来源。不要取消宿主机端口发布，不要改动数据卷。
3. 调整数据库容器后检查 /ready 返回 200，并用自己的账号验证采购、销售及库存流水页面。若看到旧前端，可刷新浏览器。
4. 后续把已验证的完整备份加密复制到另一处存储。
