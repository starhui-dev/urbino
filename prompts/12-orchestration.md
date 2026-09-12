# 阶段 12 · 完整请求链路与端到端服务

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 11 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/01-architecture.md`
- `docs/06-auth-admin.md`
- `docs/07-routing-quotas.md`
- `docs/08-protocols.md`
- `docs/09-metering-ledger.md`

## 目标与实际工作

把 public APIs 按 01 文档接成完整链路：鉴权 -> 能力 -> request/幂等 -> 配额 -> 上游与价格 -> hold/attempt -> 最后状态重查 -> 安全出站 -> stream -> usage/结算 -> release。
严格定义 defer/finalize 的异常路径；失败补偿不会漏 lease/hold；context cancel 不导致后台继续模型请求；DB cleanup 有界。
持久 response binding 后才暴露 provider ID；unknown/跨租户 continuation 出站前拒绝。
实现 public Idempotency-Key 的不重发语义、payload_digest 冲突、请求状态查询；不谎称可重放 SSE。
只有完整链路经过本阶段测试的路由才进入 production listener。模拟 provider 只允许 test/dev 显式模式。

## 交付与专属验收

交付正式公共 HTTP 服务、完整 E2E、请求状态机和排错文档。
E2E 从 CLI/API 创建租户/key/账户 -> 三类协议 mock 调用 -> 用量/余额 -> 撤销 key -> 请求拒绝；测试流式部分输出、PG 错误、客户端断开、结果未知、租约释放以及任何失败后的账务状态。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=12 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/12.md 与 evidence/stages/12.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
