# 阶段 18 · 授权核对、真实上游与灰度演练

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 17 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/00-scope.md`
- `docs/08-protocols.md`
- `docs/09-metering-ledger.md`
- `docs/12-deployment-release.md`

## 目标与实际工作

读取运营者提供的明确授权记录、测试提供商/模型/金额/次数上限、独立测试凭据与测试环境；没有这些只做测试 harness，不自动寻找或购买凭据。
逐一核对正式文档/合同支持的认证模式、第三方集成用途、配额范围、数据处理和模型能力。消费订阅不能因“已有token”就通过。
对拟启用 provider 用最少次数测试JSON/SSE、tools、usage、rate headers、取消、续接归属、计价维度；不要对真实账号主动制造封禁、恶意刷刷新或压测。
记录真实使用的客户端版本与配置；Codex/其他CLI只声明已测的HTTP功能，不臆称WS/私有接口兼容。
在自有 staging 完成小租户灰度、监控、撤销key、回滚演练。未通过的能力明确禁用；核心至少一个真实提供商通过。

## 交付与专属验收

交付 tests/live/安全harness、授权记录、原始脱敏测试证据、CLIENT_COMPATIBILITY、canary report。
缺真实凭据/网络/许可 -> BLOCKED；不能把 mock 标 live。每个生产enabled能力都有匹配版本证据；费用不超批准上限；没有真实上游通过则最终必须NO-GO。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=18 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/18.md 与 evidence/stages/18.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
