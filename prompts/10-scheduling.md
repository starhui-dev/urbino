# 阶段 10 · 路由、配额、租约与有限重试

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 09 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/01-architecture.md`
- `docs/07-routing-quotas.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

实现多层过滤、priority+weighted least-inflight、模型别名、quota_group、强资源归属与软 affinity；所有阶段保留 tenant/project/credential/egress 边界。
实现 Valkey 原子 RPM/TPM 预占、并发租约、owner ID、续租、幂等释放、有界等待与取消。
实现 account 状态机、按 model/account/org 分类的 cooldown、Retry-After 秒/日期、文档化 reset；错误不能自动解除管理停用。
定义 RetryDecision 输入含 dispatch certainty/response committed/attempt count/provider idempotency；默认不重试写后未知请求；不为 403/封禁轮换身份。
实现 Valkey epoch 恢复闭锁、PG 活跃 attempt 参照、断连取消；状态未知不能退回无限本地并发。

## 交付与专属验收

交付 routing/quota、Lua、恢复闸门与策略说明。
真实 Valkey + 两进程测试：同组织不同 key 配额共享、强 ID 不迁移、软亲和不越冷却、lease double release、续租失败、队列取消、429 reset、不安全重试、Valkey 重启闭锁。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=10 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/10.md 与 evidence/stages/10.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：路由过滤/配额/租约/冷却应用逻辑 |
| 测试切片 | urbino-tester：组织级配额/并发/重试边界测试 |
| 必须串行整合 | 主 Agent：资源归属/重试判据/共享租约 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
