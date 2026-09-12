# 阶段 13 · 完整管理 API 与 CLI

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 12 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/06-auth-admin.md`
- `docs/09-metering-ledger.md`
- `docs/10-config-operations.md`

## 目标与实际工作

补齐资源 CRUD/状态变更、列表/游标、过滤、角色、调整、停用/复核、授权 registry、策略与价格版本发布/回滚。
所有管理写支持审计、权限、幂等和版本冲突；提供安全账户导入，不开放明文秘密导出，不扫描他人 token。
实现 `urbino` 薄 CLI 调管理 API，帮助/版本/错误说明中的正式产品名保持 Urbino，不设中文名；secret-file/stdin、安全一次性输出，JSON 及可读输出；实现 doctor、config validate、ledger verify 的权限边界。
编写 Linux shell 与 PowerShell 示例，覆盖从空库到首个受控请求的全流程；不创建前端、Swagger UI 或 Node 工具链。
只读自身 usage/balance 和内部 request debug 输出均脱敏、作用域化。

## 交付与专属验收

交付完整 OpenAPI、生成 client、CLI、ADMIN_API.md 和操作示例。
黑盒测试 API 与 CLI 的同等权限、secret 不上 argv/日志、管理 key 过期/撤销、幂等创建含secret的安全响应、并发更新409/412、拒绝最后管理员被误删、分页不过租户。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=13 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/13.md 与 evidence/stages/13.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
