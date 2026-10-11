# 商品与往来单位选择体验实施记录

对应规格：`docs/master-data-selection-ux-spec.md`，GitHub #65。

实施位置：`/tmp/simple-inventory-issue65`，分支 `feat/master-data-selection`。
依赖基线提交：`b135367`，保留主工作区已有的往来概览改造；本轮没有合并回主分支。

## 已实现

- 可复用 `RecordPicker`：直接输入、多词高亮、最多六条快捷候选、宽 modal 表格与服务端分页、键盘选取、Esc 恢复、错误重试和已选摘要。
- 最近使用按当前账号与业务场景保存到当前浏览器；使用前重新读取档案并核对状态、商品类型和往来身份。
- 采购／销售列表支持关键词查询全部关联单据或指定对象精确查询，两种参数互斥，条件保留在 URL。
- 档案搜索支持多词跨字段匹配、编码／名称完全匹配优先；双库使用一致的 ASCII 大小写及字面通配符处理。
- 单据搜索可匹配当前档案与同一条商品明细的实际快照，并返回历史资料命中提示；不把不同商品行拼成一次命中，不虚构旧品牌或采购供应商描述。
- 换选保护手工成交价、明确清空的送货资料；单位变化提示核对数量，取消保持原值，同一商品／客户重选保留已有填写。
- 往来余额与明细接入单位关键词。全店概览保持全店范围；跨单位明细逐行显示单位／方向，期间对账仍要求指定单位。
- API 源注释与 Swagger 同步，无 Schema migration，无测试专用接口。

## 精简验收

用户要求“简化验收，尽快实现”，本次收敛为核心浏览器流程、双库公开 HTTP 搜索契约和项目基础门禁。

浏览器入口：`web/tests/master-data-selection.mjs`，需要已迁移并包含种子数据的隔离 API：

```sh
cd web
SELECTION_API_URL=http://127.0.0.1:18165 node tests/master-data-selection.mjs
```

脚本通过公开 API 造数，实际操作采购／销售页面；检查快捷候选数量、modal 分页与 Esc、键盘选取与数量焦点、重复选择、手工价格保护、单位变化、取消换选、关键词 URL、送货资料保护和最近使用保存，并通过公开读取核对保存草稿的商品、数量与成交价。

双库新增契约：`TestSQLiteMasterDataSelectionHTTPContract`、`TestPostgresMasterDataSelectionHTTPContract`。覆盖字面多词、相关性排序、完整分页、单据去重、不跨商品拼词、真实历史快照、参数互斥、停用及往来关键词；Taskfile 与数据库兼容 CI 的现有正则各选中一次。

最终验证结果：

- 精简真实浏览器验收通过。
- 新增 SQLite／PostgreSQL 公开 HTTP 契约分别通过。
- `task frontend:lint`、`task frontend:build` 和 `task api:docs` 通过。
- `task db:check` 中后端测试／vet、SQLite 全量契约通过；PostgreSQL 全量运行仅 `TestPostgresPartnerAttachmentBackupRestore` 失败，原因为当前环境缺少 `pg_dump`，不是本次搜索断言失败。完整门禁不能记为通过。
- `git diff --check` 通过。

验收日志保存于 `/tmp/issue65-{browser,frontend,db}-check.log`，Swagger 生成日志为 `/tmp/issue65-api-docs.log`。

本次没有开展经营者人工试用、完整浏览器异常／输入法／多账号矩阵，也没有逐个迁移或执行依赖旧选择器结构的历史浏览器专项脚本。
