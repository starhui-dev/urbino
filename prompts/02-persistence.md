# 阶段 02 · 数据库、迁移与查询边界

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 01 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/02-data-model.md`
- `docs/09-metering-ledger.md`
- `docs/11-testing.md`

## 目标与实际工作

按数据规格实现必要 migration、pgx pool 与 sqlc 查询；后续阶段新增领域表时继续版本化 migration，不用假数据填业务必需字段。
实现 tenant 复合外键、scope 查询、request/attempt 唯一性、价格版本及不可变账本基础约束。可先建立结构，但不能宣称收费已完成。
实现显式 urbino migrate，迁移锁、schema compatibility check，运行角色与 migrator 权限分离；不在每个 API 实例启动时自动迁移。
所有 query 有 context、statement/lock 超时；避免把 driver error/DSN 原文发到客户端。对涉及重试的短事务统一封装与完整回滚。
构建真实 PG 集成测试容器和本地 test DSN 安全检查，避免误连生产。

## 交付与专属验收

交付 migrations/、sql/、sqlc 配置及生成仓储、PG test helper、迁移 runbook。
真实 PG 验证 fresh migrate、重复运行、并行 migrator、复合 FK 越租户失败、唯一性、角色无 DDL、事务取消/rollback。测试 schema 升级兼容；没有 Docker/PG 不能把集成门禁标通过。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=02 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/02.md 与 evidence/stages/02.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：按已批准SQL实现PG仓储与事务端口 |
| 测试切片 | urbino-tester：真实PG唯一约束/越权/rollback测试 |
| 必须串行整合 | 主 Agent：migration序号/生成sqlc/锁顺序 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
