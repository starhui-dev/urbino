# 阶段 07 · OpenAI API Key 适配

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 06 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/04-credentials-oauth.md`
- `docs/05-transport-egress.md`
- `docs/08-protocols.md`
- `docs/09-metering-ledger.md`

## 目标与实际工作

核对并记录官方 Chat Completions、Responses、Embeddings 的请求/响应与错误协议，按已声明能力子集实现 API Key provider。不要使用 chatgpt.com 私有后端或假冒 Codex 标识。
支持非流式/流式、文本、函数工具、结构化输出已验证子集；/models 使用本地授权目录；不把 Responses 转成 Chat 伪装成功。
实现 usage 字段映射、cache/reasoning 包含关系、资源 ID 观察、rate/usage headers 安全解析和 error classifier。
拒绝 WS、background、n>1、未配置 hosted tools/媒体等未支持能力；记录客户端可见错误。
目前用安全 harness 调适配器，不开放无配额/计费的公网正式路由；完整编排在第 12 阶段。

## 交付与专属验收

交付 provider/openai、官方来源说明、合成 contract fixtures、支持字段表。
测试 Responses completed/incomplete/failed、Chat 空 choices usage 块、function tools、已知/缺失 usage、401/403/429/5xx、timeout、未知 response id 的 owner hook。无真 key 仍须完整通过 mock；live 留给第 18 阶段。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=07 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/07.md 与 evidence/stages/07.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：OpenAI已声明HTTP协议子集适配 |
| 测试切片 | urbino-tester：请求/错误/usage golden与流式夹具 |
| 必须串行整合 | 主 Agent：能力白名单/协议字段/计量定义 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
