# 阶段 17 · 容器、配置、备份还原与发布供应链

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 16 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/10-config-operations.md`
- `docs/12-deployment-release.md`

## 目标与实际工作

遵守 project.json 的镜像、Compose 服务、配置路径与二进制命名，禁止残留临时名称。生成并实际测试非root Dockerfile、内置/外部依赖 Compose、Nginx SSE 示例、生产配置、secret 文件示例与 schema；无默认口令、无公网PG/Valkey。
固化依赖/image digest、SBOM、许可证清单、安全扫描与构建 provenance；CI 最小权限，fork PR 无真实密钥。
实现应用 schema compatibility、显式迁移、readiness/drain、升级/回滚流程；禁止自动有损 down。
实际在隔离环境进行 PG+密钥备份/还原、ledger verify、权限/吊销验证；记录 RPO/RTO 的测量，不只生成脚本。
提供适配已有1Panel/反代与外部数据库的文档；管理员端口不意外随 public location 暴露。测试arm64/amd64并区分只编译与实际运行。

## 交付与专属验收

交付 deploy/、Dockerfile、配置 schema/示例、BACKUP_RESTORE、UPGRADE_ROLLBACK、供应链报告。
实际 docker compose config、build、up/health、API冒烟、SSE flush、secret权限、public/admin隔离、restore、rollback；工具不可用不能 verified。生产镜像无shell不是必要口号，以最小攻击面和实际运维需求为准。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=17 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/17.md 与 evidence/stages/17.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：容器/运维脚本按审批配置落实 |
| 测试切片 | urbino-tester：配置/备份恢复/回滚演练自动检查 |
| 必须串行整合 | 主 Agent：迁移顺序/镜像摘要/供应链发布 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
