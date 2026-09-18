# 阶段 14 · 后台任务、指标与审计运维

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 13 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/02-data-model.md`
- `docs/10-config-operations.md`
- `docs/11-testing.md`

## 目标与实际工作

实现 River 事务任务/outbox 选定方案、幂等 job handlers、有限重试、失败查询及告警；不会在任务重放中重复发模型请求。
实现用量汇总、ledger verify 调度、未终结 request/hold 升级、过期 OAuth flow 清理、授权到期检查、受控健康探测。
实现低基数 Prometheus（应用指标前缀 urbino_）、slog redaction、可选无内容 OTel（service.name=urbino）；liveness/readiness/draining 和 pprof 隔离。
实现 retention、日志轮转要求与配置、停止/重启/队列积压 runbook；所有清理不破坏账本或活跃请求引用。
实现優雅停机并记录取消/未知状态，不在收到 SIGTERM 后接新请求。

## 交付与专属验收

交付 jobs、监控/告警模板、RUNBOOK、retention/health 测试。
验证 job 重放一次效果、payload 无秘密、日志注入/超长模型低基数、Valkey/PG readiness 闭锁、worker backlog 告警、SIGTERM drain、过期清理不删账本。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=14 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/14.md 与 evidence/stages/14.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：River任务/指标/审计查询小切片实现 |
| 测试切片 | urbino-tester：重复任务/重试/敏感字段/告警验证 |
| 必须串行整合 | 主 Agent：指标基数/任务幂等/审计保留 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
