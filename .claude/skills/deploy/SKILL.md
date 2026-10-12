---
name: deploy
description: 部署、发布、上线或回退本项目的生产服务器（ubuntu@82.157.189.251 的 /opt/simple-inventory 原生部署）。用户说“部署”“发布”“上线”“升级服务器”“回退上一版本”时使用。
---

# 部署到生产服务器

目标机 `ubuntu@82.157.189.251`：`/opt/simple-inventory` 原生部署——systemd 跑 Go 二进制（内嵌前端），PostgreSQL 与 OpenResty 由 1Panel 管理。入口 `https://jxc.ezios.cn` → OpenResty → `127.0.0.1:18080`。

## 四步

版本名用 `native-$(date -u +%Y%m%dT%H%M%SZ)`，全程用同一个 `$VERSION`。

### 1. 构建

从仓库根目录执行；完成标准是 `$OUT` 里三个二进制 `file` 显示 statically linked，且有 `migrations/`、`configs/config.yaml`、`SHA256SUMS`。

```sh
VERSION=native-$(date -u +%Y%m%dT%H%M%SZ); OUT=/tmp/si-release/$VERSION
mkdir -p "$OUT"
(cd web && npm run build)
rm -rf server/internal/webui/dist; mkdir -p server/internal/webui/dist; cp -R web/dist/. server/internal/webui/dist/
LDF='-linkmode=external -extldflags=-static -s -w'; TAGS='netgo osusergo sqlite_omit_load_extension'
(cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags="embedweb $TAGS" -trimpath -ldflags="$LDF" -o "$OUT/simple-inventory-api" ./cmd/api)
(cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags="$TAGS" -trimpath -ldflags="$LDF" -o "$OUT/simple-inventory-migrate" ./cmd/migrate)
(cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags="$TAGS" -trimpath -ldflags="$LDF" -o "$OUT/simple-inventory-backup" ./cmd/backup)
rm -rf server/internal/webui/dist
cp -R server/migrations "$OUT/migrations"; mkdir -p "$OUT/configs"; cp server/configs/config.yaml "$OUT/configs/config.yaml"
(cd "$OUT" && find . -type f ! -name SHA256SUMS -printf '%P\n' | sort | xargs sha256sum > SHA256SUMS)
file "$OUT"/simple-inventory-{api,migrate,backup} | grep -c "statically linked"
echo "$VERSION"
```

### 2. 发布

完成标准：服务器上该版本目录存在且文件齐全。

```sh
rsync -a --rsync-path="sudo rsync" "$OUT/" ubuntu@82.157.189.251:/opt/simple-inventory/releases/$VERSION/
ssh ubuntu@82.157.189.251 "sudo ls /opt/simple-inventory/releases/$VERSION/"
```

### 3. 切换

`prepare` 做服务器端校验、只读迁移检查与 18081 预检，完成后打印 `PREFLIGHT_OK`；`upgrade-native.py` 加锁、停服、写切换前完整备份（库+附件+配置+旧版本）、显式 `migrate up`、换 `current`、启动、等 `/ready`，成功打印 `UPGRADE_OK <版本> from <上一版本>`。停机约十秒。

```sh
ssh ubuntu@82.157.189.251 "cd /opt/simple-inventory && sudo python3 native-tools/deploy-native.py prepare $VERSION && sudo python3 native-tools/upgrade-native.py $VERSION"
```

### 4. 验收

完成标准：三个端点都是 200。

```sh
ssh ubuntu@82.157.189.251 'curl -fsS -o /dev/null -w "health=%{http_code} " http://127.0.0.1:18080/health; curl -fsS -o /dev/null -w "ready=%{http_code} " http://127.0.0.1:18080/ready; curl -fsS -o /dev/null -w "https_ready=%{http_code}\n" https://jxc.ezios.cn/ready'
```

## 报告

只报四项：版本名、上一版本、`PREFLIGHT_OK`/`UPGRADE_OK` 结果、三个端点状态。校验清单、配置比对、资源指纹、附件与日志检查、记录文档都不在范围内。

## 范围边界

四步之外的动作先说明再等确认，包括新增或修改工具、文档、Taskfile、CI、共享配置，以及额外的校验与验收步骤。这些属于用户的维护面，不由部署顺手带来。

## 回退

`upgrade-native.py` 切换失败时会自动把 `current` 指回上一版本并重启（旧版本遇到已升级的 schema 会被 unit 的 `ExecStartPre` 迁移检查拦住，不会用旧程序读新库）。手动回退同理：

```sh
ssh ubuntu@82.157.189.251 'sudo ln -sfn /opt/simple-inventory/releases/<上一版本> /opt/simple-inventory/current && sudo systemctl restart simple-inventory.service'
```

## 细节

发布目录结构、构建参数、私有配置与备份/回退约定见 `deploy/native/README.md` 与 `deploy/native/NATIVE-DEPLOYMENT.md`；切换工具的源码见 `deploy/native/tools/`。
