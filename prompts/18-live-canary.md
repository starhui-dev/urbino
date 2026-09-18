# 阶段 18 · 授权核对、真实上游与灰度演练

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 17 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/00-scope.md`
- `docs/08-protocols.md`
- `docs/09-metering-ledger.md`
- `docs/12-deployment-release.md`

## 目标与实际工作

读取运营者提供的明确授权记录、测试提供商/模型/金额/次数上限、独立测试凭据与测试环境；没有这些只做测试 harness，不自动寻找或购买凭据。
逐一核对正式文档/合同支持的认证模式、第三方集成用途、配额范围、数据处理和模型能力。消费订阅不能因“已有token”就通过。
对拟启用 provider 用最少次数测试JSON/SSE、tools、usage、rate headers、取消、续接归属、计价维度；不要对真实账号主动制造封禁、恶意刷刷新或压测。
记录真实使用的客户端版本与配置；OMP/其他实际接入的CLI只声明已测的HTTP功能，不臆称WS/私有接口兼容。
在自有 staging 完成小租户灰度、监控、撤销key、回滚演练。未通过的能力明确禁用；核心至少一个真实提供商通过。

## 交付与专属验收

交付 tests/live/安全harness、授权记录、原始脱敏测试证据、CLIENT_COMPATIBILITY、canary report。
缺真实凭据/网络/许可 -> BLOCKED；不能把 mock 标 live。每个生产enabled能力都有匹配版本证据；费用不超批准上限；没有真实上游通过则最终必须NO-GO。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=18 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/18.md 与 evidence/stages/18.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：仅在明确测试授权内执行最小联调 |
| 测试切片 | urbino-tester：核对真实协议/用量/成本/灰度结果 |
| 必须串行整合 | 主 Agent：授权host/model/预算/部署环境 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
