# 阶段 16 · 故障、多实例与性能验证

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 15 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

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

写 progress/reports/16.md 与 evidence/stages/16.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
