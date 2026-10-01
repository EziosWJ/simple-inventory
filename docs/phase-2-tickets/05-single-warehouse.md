# 05：唯一逻辑仓库查看与维护

**GitHub issue:** [#6](https://github.com/EziosWJ/simple-inventory/issues/6) · `ready-for-agent`

## Parent

- Phase 2 基础资料规格：https://github.com/EziosWJ/simple-inventory/issues/1

## What to build

经营者进入仓库页面即可查看预置的唯一“默认仓库”，可以修改名称与备注，刷新或重启后仍读取已保存资料。没有新增、删除、停用能力。本任务独立交付迁移、认证 API、菜单、UI、审计与双库验证。

## Acceptance criteria

- [ ] PostgreSQL/SQLite schema 和 seed migration 逻辑版本同步，预置唯一默认仓库，数据库约束保证最多一份记录。
- [ ] 空库、升级和重复运行迁移验证通过，不重复建立仓库，API 启动不自动迁移或补种。
- [ ] 新增真实仓库菜单并授权 ADMIN，可查看并编辑唯一仓库的名称与备注，不依赖商品或往来单位模块。
- [ ] 名称必填且拒绝纯空白，备注选填；查询和更新使用单份资源接口，不提供多仓列表、新增、删除或停用接口。
- [ ] 页面不提供新增、删除、停用操作；成功后显示保存的资料，刷新或重启后仍保持。
- [ ] 未登录请求被拒绝，已登录 API 访问范围与确认规则一致，不以 mock 展示权限作为授权。
- [ ] 仓库修改与操作审计同事务提交，双库验证审计失败时名称和备注保持原值。
- [ ] 使用公开 HTTP 行为验证读取、更新、无效名称拒绝与持久性，并验证迁移及数据库唯一性约束。
- [ ] 完成页面查看修改演示、后端 tests+vet、前端 lint/build、双库相关检查，新增 contract 被实际执行。
- [ ] Swagger 源注解与生成产物同步，不引入库存、货位、实体地点或多仓功能。

## Blocked by

None (can start immediately).
