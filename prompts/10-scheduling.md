# 阶段 10 · 路由、配额、租约与有限重试

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 09 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

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

写 progress/reports/10.md 与 evidence/stages/10.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
