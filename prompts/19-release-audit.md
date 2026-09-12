# 阶段 19 · 独立终审与上线门禁

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 18 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/03-security.md`
- `docs/11-testing.md`
- `docs/12-deployment-release.md`

## 目标与实际工作

从最终代码 revision 重新检查所有阶段、测试证据、需求 traceability、生产配置、enabled capability、授权/live、备份还原和扫描结果。
实现真实 make release-gate，对关键缺失/失败/SKIP非零退出；拒绝模板证据、旧revision证据、只有文件名无命令输出的PASS。
全量编译测试、生成检查、race/集成/契约/E2E、必要chaos、安全与许可证扫描；不因之前通过就跳过最终受影响范围。
检查生产可达路径无stub/TODO假实现，无mock provider打开，无预置密钥，无CPA/S2A依赖，无前端，无未披露兼容降级。
写 docs/RELEASE_READINESS.md 与最终发布清单：明确 GO/NO-GO、拓扑、范围、证据、阻塞和残余风险。没有live或恢复证据不能GO。不得自动部署生产。

## 交付与专属验收

交付终审报告、可验证 release manifest、镜像/二进制摘要、操作者执行步骤和回滚步骤。
只有 checklists/RELEASE_GATES.md 的必要门禁全部满足才 verified/GO；如有阻塞，实际代码保留，输出NO-GO及可执行修复任务，不伪造成功、不要求用户重新描述已确定需求。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=19 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/19.md 与 evidence/stages/19.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
