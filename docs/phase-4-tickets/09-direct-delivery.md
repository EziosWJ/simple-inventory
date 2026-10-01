# 09：直送采购销售关联、数量核对与取消顺序

**GitHub issue:** [#23](https://github.com/EziosWJ/simple-inventory/issues/23) · `ready-for-agent`

## Parent

Phase 4 规格：https://github.com/EziosWJ/simple-inventory/issues/14

## What to build

经营者把直送采购和销售关联为同一笔业务，查看两边状态，先采购后销售分别过账；取消先处理销售，历史关联保留。

## Acceptance criteria

- [ ] 两边表单/详情交付直送标记、关联操作及可打开的对应单据，普通备货采购/销售无需关联。
- [ ] 一张直送采购最多关联一张未取消销售单，取消旧关联后可以重新关联，历史记录不抹除；一张销售关联一张采购。
- [ ] 实物商品内部 ID 与汇总数量一致，价格/行数可不同，销售可另含服务；按商品合计核对，不限制逐行一一相等。
- [ ] 销售过账事务内验证关联采购 POSTED、关联唯一性及实物汇总一致；关联草稿改动、采购取消及并发竞争不能绕过核对。
- [ ] 采购未过账拒绝直送销售；分别过账允许中间库存暂增，一侧失败明确显示该侧状态，不宣称两单联合生效。
- [ ] 取消关联采购前须先取消未取消销售，生效退货约束沿用原单规则；旧单号/关联/操作历史仍可追溯。
- [ ] 双库 HTTP 覆盖分行价格不同、服务附加、数量不一致、未过账采购、并发关联/取消竞争和防重；页面演示完整直送正向路径。
- [ ] 沿用 JWT 和已登录账号业务范围，实际菜单按角色接入并授予 ADMIN，拒绝未登录请求；不以 mock 权限替代后端认证。
- [ ] 所需 schema/seed 使用 PostgreSQL/SQLite 逻辑版本同步的增量 Goose 迁移，不改旧版本、不用 AutoMigrate；保持现有业务数据与 API 兼容。
- [ ] 本任务业务维护与操作审计同事务，错误明确且保留表单输入；新增/修改 REST API 源注解与 Swagger 重新生成同步。
- [ ] 以公开 HTTP 和真实双库验证本任务行为，新增 contract 名含 Postgres/SQLite 并确实被门禁执行；相关后端 tests/vet、前端 lint/build 和数据库检查完成，报告失败、跳过和未执行项。

## Blocked by

- #21：https://github.com/EziosWJ/simple-inventory/issues/21
