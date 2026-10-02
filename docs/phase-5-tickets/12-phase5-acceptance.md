# 12：整阶段技术验收、打印回归与经营者试用准备

**GitHub issue:** [#51](https://github.com/EziosWJ/simple-inventory/issues/51) · `ready-for-agent`

## Parent

[Phase 5 规格 #39](https://github.com/EziosWJ/simple-inventory/issues/39)

## What to build

交付已贯通的 Phase 5 技术验收证据、真实打印回归和父母代表性业务试用材料，明确技术、恢复与人工试用各自状态。

## Acceptance criteria

- [ ] 两个真实会话由首页完成采购、销售、搜索、草稿/过账、查欠款和往来、来源追溯及查询上下文恢复。
- [ ] 覆盖超过 500 条/同名/停用/身份变化资料、未保存离开、版本冲突及确认加载、新建/编辑/过账成功后丢响应和安全重试。
- [ ] 双库以公开 HTTP 验证幂等、版本、回滚、业务/回执/审计事务与原业务规则；库存与往来各自与流水累计一致，服务无库存。
- [ ] 送货单金额显隐、草稿标记、历史快照含空值、多页签收及对账完整期间/冲销真实 PDF 回归，核对 A4、页码、边界和末页合计。
- [ ] 双库空库及 Phase 4 增量升级、REST 文档源/生成同步、新契约前缀及 Taskfile/CI 各选一次核对。
- [ ] task check、task db:check、task build:check 及最终隔离 HTTPS 部署、每日备份、整套恢复/升级回退演练通过或明确记录失败/跳过/未执行项。
- [ ] 提供两名经营者独立登录后代表性业务的试用清单、结果记录及困难反馈方式，不发送外部通知或操作正式服务器。
- [ ] 自动技术验收与人工试用分别记录；人工尚未执行时明确待人工，不勾为通过、不宣称阶段适合正式启用，父母反馈后由相关任务修正。
- [ ] 不重复建设已有 Phase 4 功能、不关闭或改动旧 issue；只修复本阶段公开行为的真实缺口并保留已有未提交改动。

## Blocked by

- [T08：工作台常用业务入口与双账号贯通（#47）](https://github.com/EziosWJ/simple-inventory/issues/47)
- [T11：整套恢复与升级回退演练（#50）](https://github.com/EziosWJ/simple-inventory/issues/50)
