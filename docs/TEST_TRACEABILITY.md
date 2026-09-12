# 阶段 00 测试追踪

| test_id | 实际测试/检查 | 证据 |
|---|---|---|
| P00-T01 | `go test ./...`、`go vet ./...`、`go build -o urbino.exe ./cmd/urbino` | `evidence/raw/go-test.txt`、`go-vet.txt`、`go-build.txt` |
| P00-T02 | `go list -m all`；检查 `toolchain/dependencies.lock` 与 CPA/Sub2API reference revision | `evidence/raw/go-list-modules.txt` |
| P00-T03 | 配置样例静态检查；`internal/securitylog.TestLoggerDoesNotSerializeErrorText` | `configs/urbino.example.yaml`、`internal/securitylog/securitylog_test.go` |
| P00-T04 | `cmd/urbino` version/unknown/health 单测及真实 listener 检查 | `cmd/urbino/main_test.go`、`evidence/raw/cli-health.txt` |

# 阶段 01 测试追踪

| test_id | 实际测试/检查 | 证据 |
|---|---|---|
| P01-T01 | `internal/domain.TestMoneyBoundaries`：负数、溢出、非法币种、币种不一致与非负减法 | `internal/domain/domain_test.go`、`evidence/raw/stage01-go-test.txt` |
| P01-T02 | `internal/domain.TestRequestTerminalCannotReopen`：未知状态与终态重开拒绝 | `internal/domain/domain_test.go`、`evidence/raw/stage01-go-test.txt` |
| P01-T03 | `api.TestAdminOpenAPISourceContract` 与 `go generate ./api`：OpenAPI YAML 解析、`info.title=Urbino`、路径/管理安全方案及重复生成哈希一致 | `api/admin_openapi_test.go`、`api/generate.go`、`api/admin_types.gen.go`、`evidence/raw/stage01-go-test.txt`、`stage01-generate.txt` |
| P01-T04 | `internal/config.TestStrictYAML`、`TestProductionGuards`、`TestEnvironmentContract`；两份样例执行 `urbino config validate` | `internal/config/config_test.go`、`configs/urbino.schema.json`、`evidence/raw/stage01-config-dev.txt`、`stage01-config-prod.txt` |
| P01-T05 | `internal/protocol.TestDecodeStrict*` 与 `TestValidateObjectFields*`：重复键、深度、尾随、UTF-8、大小和危险嵌套字段 | `internal/protocol/json_test.go`、`internal/protocol/policy_test.go`、`evidence/raw/stage01-go-test.txt` |
| P01-T06 | `internal/protocol.TestCapabilityAuthAndDisabledAreDistinct`、endpoint 字段策略测试；所有公共能力默认 disabled | `internal/protocol/capability.go`、`policy.go`、`evidence/raw/stage01-go-test.txt` |

# 阶段 02 测试追踪

| test_id | 实际测试/检查 | 证据 |
|---|---|---|
| P02-T01 | migration 静态测试检查复合 tenant 外键；真实跨租户插入因本机无 PostgreSQL 未运行 | `migrations/migration_test.go`、`evidence/raw/stage02-go-test.txt`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T02 | migration 静态测试检查 request/attempt/settlement 业务键唯一约束；真实数据库冲突断言未运行 | `migrations/migration_test.go`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T03 | goose session advisory lock 已接入 `internal/storage/postgres.Migrate`；并行 migrator 未运行 | `internal/storage/postgres/postgres.go`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T04 | migration 对 `urbino_runtime` 使用最小授权且账本触发器不可变；角色权限实测未运行 | `migrations/001_initial.sql`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T05 | `RunTx` 对 context 取消执行回滚并归还连接的实现；真实取消/回滚集成未运行 | `internal/storage/postgres/postgres.go`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T06 | `CheckSchema` 对版本不兼容返回稳定错误且启动不迁移；真实 schema 升级未运行 | `internal/storage/postgres/postgres_test.go`、`evidence/raw/stage02-integration-not-run.txt` |
| P02-T07 | `TestRedactDSN`、`tests/pgtest.TestValidateDSNRejectsUnsafeTargetsAndRedacts` 验证 DSN/driver 错误不回显秘密 | `internal/storage/postgres/postgres_test.go`、`tests/pgtest/dsn_test.go`、`evidence/raw/stage02-go-test.txt` |
