# 09：Linux 单机 HTTPS 部署与账号初始化

**GitHub issue:** [#48](https://github.com/EziosWJ/simple-inventory/issues/48) · `ready-for-agent`

## Parent

[Phase 5 规格 #39](https://github.com/EziosWJ/simple-inventory/issues/39)

## What to build

维护者在隔离 Linux 环境用生产 Compose 启动内嵌前端的 API、PostgreSQL 和 HTTPS 代理，执行显式迁移，准备两名账号并验证持久性。

## Acceptance criteria

- [ ] 交付生产 Compose、配置模板和明确启动步骤，使用可识别的发布版本/镜像，API 内嵌前端，不依赖 Vite 或 Node 运行服务。
- [ ] 显式选择生产环境并关闭 Swagger；HTTPS 代理与可信代理设置一致，API/数据库在内部网络，说明域名/证书及续期准备。
- [ ] migration 为显式的一次性步骤，成功后业务服务可就绪；空库和旧版本升级路径可执行，失败不放行、不启动 AutoMigrate。
- [ ] 数据库、上传文件及必要配置使用持久存储，API 非 root 所需权限正确；重建容器后登录和已保存业务/文件保持。
- [ ] 提供受控首次初始化流程，更换默认管理员凭据、创建两名独立业务账号并配置已有角色菜单；不新增岗位权限模型。
- [ ] 存活/数据库就绪及日志检查明确；配置示例无真实凭据，正常输出不泄露密码和密钥。
- [ ] 隔离环境实际验证 HTTPS 浏览器/API、页面深链接刷新、就绪、两个账号共享业务及生产 Swagger 关闭；测试证书不宣称为正式证书。
- [ ] 生产内嵌构建及相关检查通过；技术可独立准备，正式服务器/DNS/真实证书操作仍另行安排，遵守阶段顺序发布最终版本。

## Blocked by

None (can start immediately).
