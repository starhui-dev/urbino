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
