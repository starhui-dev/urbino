# 阶段 08 · Anthropic 与 Gemini 原生适配

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 07 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/05-transport-egress.md`
- `docs/08-protocols.md`
- `docs/09-metering-ledger.md`

## 目标与实际工作

实现 Anthropic API-key Messages/count_tokens 和 Gemini API-key generateContent/streamGenerateContent/countTokens；保持各自原生协议，不做跨协议映射。
各自定义认证注入、必要版本参数、工具事件、结束原因、usage 归一化、错误分类和额度范围解析。
Anthropic 累计计量与缓存字段、Gemini usageMetadata 不使用通用“全部相加”；写 provider-specific 映射表与算例。
所有上游原始错误严格有界并转换安全错误，不暴露 key；不实现 Claude.ai 消费 OAuth、Vertex 私有 IAM 或未知 beta。
生产能力默认 disabled_until_live，mock 完成不能替代真实联调。

## 交付与专属验收

交付两个 provider 包、contract fixtures、能力与 usage 映射文档。
测试各协议 JSON/SSE、tool use、empty/stop、安全拒绝、unknown event、计量累计和缺失、超大事件、模型参数不支持；确认两个提供商没有偷偷走 OpenAI Chat 转换。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=08 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/08.md 与 evidence/stages/08.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：Anthropic和Gemini分别实现独立适配切片 |
| 测试切片 | urbino-tester：各自原生协议/流式/usage契约测试 |
| 必须串行整合 | 主 Agent：共享类型改动/能力与计价映射 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
