# 阶段 11 · 计量、预留与账本结算

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 10 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/02-data-model.md`
- `docs/09-metering-ledger.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

实现 typed Usage、缺失/部分/已知标记和累计/增量合并；每个 provider 的 input/cache/output/reasoning 语义分开。
实现不可变价格发布、微单位金额、精确计算/舍入、请求预算上界；预付费无可信上界则拒绝，而不是凭粗估放行。
实现 hold 预留、幂等 settlement、平衡 journal、余额投影、审计调整、pending_reconciliation 和差额运营损失策略。
新增数据库级账本约束/写权限，确保普通应用路径不能写不平衡或直接编辑历史；固定锁序和事务 retry。
实现 ledger verify、查询投影重建和 unknown usage 人工处理接口的应用层；不与在线支付集成。

## 交付与专属验收

交付 billing/metering、migration、真实 PG 并发/property tests 和计价 golden。
验证测试价算例 4500 微美元；并发抢余额、重复 usage/terminal/job、价格中途变更、舍入/溢出/负值、多币种拒绝、结算前后崩溃、缺失 usage 不等于0、超 hold 不造成无授权欠款。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=11 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/11.md 与 evidence/stages/11.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：计量/预留/结算按明确账本契约实现 |
| 测试切片 | urbino-tester：PG并发/幂等/舍入/unknown usage测试 |
| 必须串行整合 | 主 Agent：金额类型/锁序/平衡约束/迁移 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
