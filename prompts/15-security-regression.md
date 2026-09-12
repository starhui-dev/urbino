# 阶段 15 · 安全审查与回归完整化

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 14 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/03-security.md`
- `docs/11-testing.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

按 checklists/test-matrix.csv 和威胁表做独立攻击面审查，补齐每项实际测试名与证据，不用单个空测试覆盖全部要求。
针对入口头、JSON duplicate/深度、URL、跨租户、credentials、log/trace、错误正文、回调、权限升级及账本进行负向测试；只针对自有环境。
运行 race/fuzz-smoke/govulncheck/静态安全与依赖秘密扫描；核对 scanner 输出和误报，不把无扫描工具标通过。
审查所有 provider 的网络调用是否统一受控，默认 Transport、GetBody、隐式重试、测试例外是否逃入生产。
修复 P0/P1；不能降低检查标准或删测试过关。出 SECURITY_REVIEW.md，说明覆盖和未覆盖范围。

## 交付与专属验收

交付安全修复、完整 traceability、扫描日志与独立审查记录。
关键包覆盖目标85%加全部必要负向用例；若工具/权限缺失为 BLOCKED。检查源码/镜像/样例/文档无实际密钥，且没有客户端身份伪装或挑战绕过逻辑。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=15 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/15.md 与 evidence/stages/15.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
