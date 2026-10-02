# 08：工作台常用业务入口与双账号贯通

**GitHub issue:** [#47](https://github.com/EziosWJ/simple-inventory/issues/47) · `ready-for-agent`

## Parent

[Phase 5 规格 #39](https://github.com/EziosWJ/simple-inventory/issues/39)

## What to build

两名经营者分别登录后能在工作台直接开采购/销售单、查欠款或查单据，访问同一店铺业务，并保留各自实际操作人。

## Acceptance criteria

- [ ] 工作台提供开采购单、开销售单、查欠款、查单据四类明确入口，分别到已交付的完整页及查询路径。
- [ ] 查单据入口可选择采购/销售；工作台不添加虚构统计数据、经营报表或跨业务全局搜索。
- [ ] 入口与真实菜单角色配置一致，两名业务账号均能完成日常操作；后端沿用登录访问范围，不按创建人隐藏业务。
- [ ] 两账号可看同一草稿、原单、余额和往来；保存、过账及冲突恢复显示真实执行人。
- [ ] 登录/未登录、直接路由刷新、菜单搜索及返回行为正确，既有系统管理规则不被 mock 显隐替代。
- [ ] 使用现有设计模式和基础组件，避免整体无关改版；需要菜单 seed 时使用双库增量迁移并同步断言。
- [ ] 真实浏览器两个会话完成从首页录单、查账、原单追溯及冲突处理，前端 lint/build 和相关后端/数据库检查通过。

## Blocked by

- [T03：采购保存并过账与操作结果核实（#42）](https://github.com/EziosWJ/simple-inventory/issues/42)
- [T05：销售保存并过账与异常恢复（#44）](https://github.com/EziosWJ/simple-inventory/issues/44)
- [T06：采购销售单据检索与历史停用资料查询（#45）](https://github.com/EziosWJ/simple-inventory/issues/45)
- [T07：欠款查账、往来明细与来源追溯体验（#46）](https://github.com/EziosWJ/simple-inventory/issues/46)
