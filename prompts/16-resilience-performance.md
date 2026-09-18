# 阶段 16 · 故障、多实例与性能验证

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 15 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/07-routing-quotas.md`
- `docs/09-metering-ledger.md`
- `docs/12-deployment-release.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

实现两个独立进程的 E2E/chaos harness，共享真实 PG/Valkey，执行 13 文档 A-J 演练，不把 goroutine 当多实例。
测试 refresh 崩溃、PG 结算窗口、Valkey 状态丢失/epoch、stream 中途 kill、worker 重放、时钟/续租、代理故障与 DNS 重绑定。
实现可复现 mock 压测和直连 mock baseline，记录环境、时长、并发、RSS/CPU/GC、p95/99、TTFT、错误/拒绝；达到设计目标或明确记录差距。
观察泄漏、死锁与 unbounded queues；根据证据优化，不更换架构掩盖逻辑漏洞。
报告网络分区与上游取消不可控的剩余边界，不声明不可能证明的端到端 exactly-once。

## 交付与专属验收

交付 chaos/bench 工具与真实报告，全部可复现命令。
必需断言：无越权/重复扣费、无旧token自动重放、恢复闸门有效、并发计数不从空状态错误重开、资源不持续增长。若硬件不同，报告差异不伪造基线；安全不变量失败阻止后续上线。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=16 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/16.md 与 evidence/stages/16.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：按主Agent分析修复测量发现的问题 |
| 测试切片 | urbino-tester：故障注入/多实例/有界压测脚本 |
| 必须串行整合 | 主 Agent：测试拓扑/资源预算/性能结论 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
