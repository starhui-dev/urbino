# 阶段 00 · 依赖核对

核对日期：2026-09-18。版本均来自本机 Go module proxy 的实际 `go list -m` / `go mod download` 返回；不是 `latest` 字面量写入构建。代码当前只直接导入 `gopkg.in/yaml.v3`；其余条目是已选技术栈的锁定候选，待对应阶段首次引入时仍需重跑兼容性与安全核对。

## 当前 module

| 依赖 | 锁定版本 | 状态 | 许可证 | 兼容性/维护事实 |
|---|---:|---|---|---|
| Go toolchain | 1.27.1 | 已实测 | Go toolchain license | `go version` 实测为 `go1.27.1 linux/amd64`；go.mod 固定 `go 1.27.1` |
| gopkg.in/yaml.v3 | v3.0.1 | 直接依赖，已写入 go.mod/go.sum | MIT + Apache-2.0（模块 LICENSE 实读） | `go list -m -json` 返回版本时间 2022-05-27；用于受限启动配置，首次引入已通过 go test/vet/build |

## 已选栈候选锁定

这些版本已通过实际模块查询和源码 LICENSE 文件读取；未在阶段 00 的二进制中导入，避免为未实现功能增加依赖面。摘要和 go.sum 只对已导入模块负责。

| 依赖 | 查询版本 | 许可证（LICENSE 实读） | 来源/后续用途 |
|---|---:|---|---|
| github.com/go-chi/chi/v5 | v5.3.2 | MIT | HTTP 路由；阶段 01+ |
| github.com/jackc/pgx/v5 | v5.11.0 | MIT | PostgreSQL 驱动；阶段 02+ |
| github.com/pressly/goose/v3 | v3.28.0 | MIT | 数据库迁移；阶段 02+ |
| github.com/valkey-io/valkey-go | v1.0.78 | Apache-2.0 | Valkey 客户端；阶段 02+ |
| github.com/riverqueue/river | v0.47.0 | MPL-2.0 | PostgreSQL worker；阶段 12+ |
| github.com/sqlc-dev/sqlc | v1.31.1 | MIT | SQL 代码生成工具；阶段 02+ |
| github.com/oapi-codegen/oapi-codegen/v2 | v2.8.0 | Apache-2.0 | OpenAPI 生成；阶段 01+ |
| github.com/prometheus/client_golang | v1.24.1 | Apache-2.0 | 指标；阶段 14+ |
| go.opentelemetry.io/otel | v1.46.0 | Apache-2.0 | 可选 tracing；阶段 14+ |

## 锁定与限制

- `go.mod`/`go.sum` 只包含当前源码真实导入的运行时模块；生成工具只通过带精确版本的 `go:generate` 命令使用，不伪装成运行时依赖。
- 具体版本、Go 最低版本、许可证变更和安全公告在首次引入、升级及发布阶段重新核对；不接受未审查的自动升级。
- 禁止 CPA、CPA SDK、Sub2API、NewAPI 作为 module、SDK、submodule 或实现来源；`go list -m all` 是阶段 P00-T02 的门禁。
- PostgreSQL/Valkey 服务版本属于运行环境，不在本地 module lock 中；部署阶段另行记录镜像 digest、备份/还原和兼容性证据。

## 阶段 01 实际引入

| 依赖 | 锁定版本 | 状态 | 许可证 | 用途 |
|---|---:|---|---|---|
| github.com/oapi-codegen/runtime | v1.7.0 | 直接依赖，已写入 go.mod/go.sum | Apache-2.0 | 生成管理契约的 UUID 类型 |
| github.com/google/uuid | v1.6.0 | 间接依赖，已写入 go.mod/go.sum | BSD-3-Clause | oapi-codegen runtime 的 UUID 支持 |
| github.com/oapi-codegen/oapi-codegen/v2 | v2.8.0 | `go:generate` 工具版本锁定，未作为运行时依赖 | Apache-2.0 | 从 `api/admin.openapi.yaml` 生成 `api/admin_gen.go` |

阶段 01 的 OpenAPI 生成实际执行 `go run ...@v2.8.0`；生成文件声明版本为 v2.8.0，第二次生成由 `make generate-check` 验证无差异。

## 阶段 02 实际引入

| 依赖 | 锁定版本 | 状态 | 许可证 | 用途 |
|---|---:|---|---|---|
| github.com/jackc/pgx/v5 | v5.11.0 | 直接依赖，已写入 go.mod/go.sum | MIT | PostgreSQL pool、事务与迁移连接 |
| github.com/sqlc-dev/sqlc | v1.31.1 | `go:generate` 工具版本锁定，未作为运行时依赖 | MIT | 从 `sqlc.yaml` 生成 PostgreSQL 查询仓储 |

阶段 02 的 pgx 与 sqlc 版本来自实际 module proxy 下载；`go generate ./internal/storage/postgres` 已实际生成查询代码。goose 仍为后续候选，当前迁移 runner 使用仓库内显式 advisory-lock/history 实现，不引入未使用运行时依赖。
