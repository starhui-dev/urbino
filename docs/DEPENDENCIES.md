# Urbino 依赖核对与锁定

核对日期：2026-09-12。Go 工具链为 `go1.27.1 windows/amd64`。当前阶段只把 chi 引入可编译模块；其他选定组件记录精确版本，待对应阶段接入并由 `go mod tidy` 更新校验和。

| 组件 | 锁定版本 | 用途 | 许可证/维护核对 |
|---|---|---|---|
| Go | 1.27.1 | 编译工具链 | 官方发行版，见 references/SOURCES.md R20 |
| github.com/go-chi/chi/v5 | v5.3.2 | net/http 路由 | MIT；官方仓库/模块可解析 |
| gopkg.in/yaml.v3 | v3.0.1 | 严格配置 YAML 解码 | MIT；阶段 01 已接入并锁定 |
| github.com/oapi-codegen/runtime | v1.7.0 | oapi-codegen 生成管理契约类型的运行时格式 | Apache-2.0；阶段 01 已接入并锁定 |
| github.com/jackc/pgx/v5 | v5.11.0 | PostgreSQL 驱动（后续阶段） | MIT；官方模块可解析 |
| sqlc | v1.31.1 | SQL 代码生成（后续阶段） | MIT；官方文档 R24 |
| github.com/pressly/goose/v3 | v3.28.0 | 迁移（后续阶段） | BSD-3-Clause；官方仓库可解析 |
| github.com/valkey-io/valkey-go | v1.0.77 | Valkey 客户端（后续阶段） | Apache-2.0；官方仓库可解析 |
| riverqueue.com/river | v0.47.0 | PostgreSQL 任务（后续阶段） | Mozilla Public License 2.0；官方文档/模块可解析 |
| github.com/oapi-codegen/oapi-codegen/v2 | v2.8.0 | OpenAPI 生成（后续阶段） | Apache-2.0；官方仓库可解析 |
| github.com/prometheus/client_golang | v1.24.1 | 指标（后续阶段） | Apache-2.0；官方仓库可解析 |
| go.opentelemetry.io/otel | v1.47.0 | 可选追踪（后续阶段） | Apache-2.0；官方模块可解析 |

未引入 CPA、CPA SDK、Sub2API 或其 SDK。版本必须在实际接入阶段再次执行 `go list -m -versions` 和许可证核对；未使用的组件不应提前加入生产依赖图。
