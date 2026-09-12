# 阶段 00 · 工程基础与依赖锁定

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 无 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/00-scope.md`
- `docs/01-architecture.md`
- `docs/11-testing.md`

## 目标与实际工作

检查空仓库或既有文件，不删除用户改动。建立可编译 Go module；真实托管路径从已确认的仓库 remote 取得，没有时使用 `example.com/urbino` 并记录待确认的托管位置，不重新命名项目、不创建远程仓库。实现 `urbino version` / `urbino serve` 最小命令；只有内部 health，没有裸露模型调用。
核对 Go 基线及每个选定依赖真实存在、许可证、维护状态和兼容性，生成 docs/DEPENDENCIES.md、toolchain/依赖锁定清单；通过官方来源取得精确版本，不猜测未来版本。
再次打开 references/SOURCES.md 中的 CPA/S2A 路径，只提取安全设计依据与反例；能访问 Git 时锁定实际 commit，不能访问就写未锁定并列阻塞，不编 SHA。不把参考仓库作为 module/submodule/SDK 引入。
建立 Makefile、基础 CI、.gitignore/.dockerignore，排除 .env、秘密、私有测试输出；建立格式、vet、build、test 的真实命令。建立 test helper、Clock 接口和结构化安全日志最小实现。
确认已有提示词文件不被 go generate 或清理脚本误删。写第一份架构 ADR：模块化单体和无前端。

## 交付与专属验收

交付 go.mod/go.sum、cmd/urbino、最小配置/健康测试、Makefile/CI、依赖记录、ADR-0001。测试未知命令非零退出、版本可读、健康 listener 默认不暴露管理；版本/帮助产品名为 Urbino，构建入口与产物分别为 cmd/urbino 与 urbino（Windows 为 urbino.exe），自有环境变量使用 URBINO_。
必须实际运行 go test ./...、go vet ./...、go build ./cmd/urbino；工具链缺失记录 NOT_RUN，不伪造。用 go list -m all 检查无 CPA/S2A 依赖。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=00 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/00.md 与 evidence/stages/00.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
