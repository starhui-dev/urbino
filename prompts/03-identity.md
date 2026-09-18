# 阶段 03 · 租户、用户、API Key 与管理鉴权

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 02 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
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

由主 Agent 写 progress/reports/03.md 与 evidence/stages/03.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：租户用户Key与管理鉴权应用逻辑 |
| 测试切片 | urbino-tester：越权/撤销/租户混淆/并发测试 |
| 必须串行整合 | 主 Agent：身份边界/秘密摘要策略 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
