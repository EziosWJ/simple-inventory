# 06：经营者打印资料统一维护与审计

**GitHub issue:** [#20](https://github.com/EziosWJ/simple-inventory/issues/20) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者在设置入口维护自己的名称、电话和地址，保存后刷新仍保持，为后续新送货单带入抬头和联系方式。

## Acceptance criteria

- [ ] 真实设置入口、认证单份资源查询/更新 API、表单交付；维护经营者名称/电话/地址，沿用现有基础配置能力，不建立多公司/多组织。
- [ ] 经营者名称必填拒绝空白，联系方式可为空且含长度校验；更新成功读取持久化结果，失败明确且保留输入。
- [ ] 维护与审计原子保存，实际操作人可追踪；权限沿用已登录业务范围及真实菜单规则。
- [ ] 提供后续草稿带入/销售过账冻结所需的读取边界，资料变更不通过回填已过账销售单修改历史。
- [ ] 双库覆盖读取、更新、重启/刷新持久化、无效输入和审计失败回滚；页面可独立演示。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

None (can start immediately).
