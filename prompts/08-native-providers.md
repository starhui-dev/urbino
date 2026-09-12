# 阶段 08 · Anthropic 与 Gemini 原生适配

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 07 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

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

写 progress/reports/08.md 与 evidence/stages/08.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
