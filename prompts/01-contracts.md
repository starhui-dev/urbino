# 阶段 01 · 领域类型、接口与配置契约

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 00 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/00-scope.md`
- `docs/01-architecture.md`
- `docs/02-data-model.md`
- `docs/06-auth-admin.md`
- `docs/08-protocols.md`

## 目标与实际工作

定义 tenant/project/principal/request/attempt/usage/money/credential-version/能力的强类型，不用 map[string]any 作为所有业务接口。
制定管理 OpenAPI（info.title=Urbino）、公共错误结构、配置 schema（urbino.yaml / URBINO_）、capability descriptor、permission scopes；使用 oapi-codegen 生成受版本锁定的类型。公共 LLM 流式接口手工 handler，不用管理 JSON writer 包装。
定义 provider 适配端口、安全 TransportFactory、Vault、UsageObserver、Reservation、Scheduler；接口保持当前需求最小，不建立几十个空实现。
明确每个 endpoint 的 allowed/forbidden/bounded-pass-through 字段策略。配置未知项报错；开发例外不可进入 production。
建立 money parse/format、UTC 时间、UUID、状态转移和安全 error 类型；统一 distinguish unsupported / unauthorized / unknown。

## 交付与专属验收

交付 api/admin.openapi.yaml、配置 JSON Schema、领域类型及校验器、能力矩阵与契约文档。生成文件可重复，第二次 generate 后无 diff。
测试负数/溢出金额、未知 enum、非法状态转移、未知配置、重复 JSON key、未经声明的 API；OpenAPI 验证器实际通过。接口未完成不能返回假 200。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=01 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/01.md 与 evidence/stages/01.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
