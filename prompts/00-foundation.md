# 阶段 00 · 工程基础与依赖锁定

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；本阶段无前置阶段。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/00-scope.md`
- `docs/01-architecture.md`
- `docs/11-testing.md`

## 目标与实际工作

检查空仓库或既有文件，不删除用户改动。按 project.json 建立可编译 Go module（尚无已确认真实仓库时使用 example.com/urbino），入口 cmd/urbino；实现 urbino version / serve 最小命令、urbino.yaml 配置入口与 URBINO_ 环境变量约定；只有内部 health，没有裸露模型调用。
核对 Go 基线及每个选定依赖真实存在、许可证、维护状态和兼容性，生成 docs/DEPENDENCIES.md、toolchain/依赖锁定清单；通过官方来源取得精确版本，不猜测未来版本。
再次打开 references/SOURCES.md 中的 CPA/S2A 路径，只提取安全设计依据与反例；能访问 Git 时锁定实际 commit，不能访问就写未锁定并列阻塞，不编 SHA。不把参考仓库作为 module/submodule/SDK 引入。
在 ADR-0001 记录已确认项目名 Urbino，不把包版本当成程序发布版本。建立 Makefile、基础 CI、.gitignore/.dockerignore，排除 .env、秘密、私有测试输出；建立格式、vet、build、test 的真实命令。建立 test helper、Clock 接口和结构化安全日志最小实现。
确认已有提示词文件不被 go generate 或清理脚本误删。写第一份架构 ADR：模块化单体和无前端。

## 交付与专属验收

交付 go.mod/go.sum、cmd/urbino、最小配置/健康测试、Makefile/CI、依赖记录、ADR-0001。测试未知命令非零退出、版本可读、健康 listener 默认不暴露管理。
必须实际运行 go test ./...、go vet ./...、go build ./cmd/urbino；工具链缺失记录 NOT_RUN，不伪造。用 go list -m all 检查无 CPA/S2A 依赖。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=00 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/00.md 与 evidence/stages/00.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：命令、配置与内部health的最小可编译切片 |
| 测试切片 | urbino-tester：命令退出码/配置错误/health隔离测试 |
| 必须串行整合 | 主 Agent：Go module/工具版本/CI与命名 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
