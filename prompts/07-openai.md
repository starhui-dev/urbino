# 阶段 07 · OpenAI API Key 适配

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 06 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

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

写 progress/reports/07.md 与 evidence/stages/07.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
