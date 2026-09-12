# 阶段 03 · 租户、用户、API Key 与管理鉴权

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 02 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/02-data-model.md`
- `docs/03-security.md`
- `docs/04-credentials-oauth.md`
- `docs/06-auth-admin.md`

## 目标与实际工作

实现管理员 bootstrap 的唯一创建、独占安全输出、重复执行保护。管理与 public listener 分离；创建最小管理员认证能力供后续资源接入。
实现 tenant/user/project/member、public Key 创建/验证/撤销/过期/scopes；public_id 查找与 HMAC digest 恒定时间验证。密钥生成使用 crypto/rand。
管理角色不得由请求 body 提升；跨租户和跨项目资源校验放在应用层与仓储双重边界。用户 Key 可关联多个模型权限。
实施认证失败入口限流、重复/冲突认证头拒绝、URL key 拒绝；可信代理边界未实现前只用 RemoteAddr，不误信 XFF。
定义 auth cache 的 5 秒撤销上界和失败关闭策略；后续 Valkey 阶段补齐多实例测试，不能先假设 Pub/Sub 必达。

## 交付与专属验收

交付鉴权中间件、最小租户/Key 管理接口、RBAC、审计写入和撤销测试。
负向测试 public key 访问 admin、管理员越 scope、跨 tenant/project、过期、撤销、双 bootstrap、重复 key id；确认日志/列表/错误无秘密。至少两个 key 可共享同一个允许模型策略。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=03 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/03.md 与 evidence/stages/03.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
