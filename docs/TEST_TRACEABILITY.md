# 测试追踪

本文件只记录已实际运行的阶段测试映射；未实现阶段保持在 `checklists/test-matrix.csv` 的 `not_started` 状态。证据均来自当前工作树的真实命令记录。

| Test ID | 实际测试/断言 | 源码路径 | 证据 |
|---|---|---|---|
| P00-T01 | `go test ./...`、`go vet ./...`、`go build ./cmd/urbino`；Makefile `fmt/vet/test/build` | `cmd/urbino/`、`internal/`、`tests/foundation/`、`Makefile` | `evidence/commands/command-summary.txt`、`evidence/commands/go-test.txt`、`evidence/commands/go-vet-result.txt`、`evidence/commands/go-build-result.txt` |
| P00-T02 | `go list -m all`、依赖版本/许可证/兼容性核对；CPA/S2A 禁止依赖扫描 | `go.mod`、`go.sum`、`docs/DEPENDENCIES.md`、`toolchain/dependencies.lock` | `evidence/commands/go-list-mod.txt`、`evidence/commands/dependency-metadata.txt`、`evidence/commands/cpa-s2a-scan.txt` |
| P00-T03 | 凭据形状、默认管理员口令与私有配置静态扫描；示例配置检查 | `configs/urbino.example.yaml`、`.gitignore`、`.dockerignore`、`internal/observability/` | `evidence/commands/secret-scan.txt` |
| P00-T04 | version/unknown command（含 `--help`）、配置 precedence/失败语义、多文档拒绝、health JSON/GET-only/路由黑盒测试与真实进程 smoke | `internal/cli/`、`internal/config/`、`internal/httpapi/`、`tests/foundation/foundation_test.go` | `evidence/commands/foundation-blackbox.txt`、`evidence/commands/cli-smoke.txt`、`evidence/commands/health-smoke.txt`、`evidence/commands/health-bind.txt` |

阶段 00 的独立 review 记录另存于 `progress/reports/00-review.md`；review 通过前阶段状态保持 `implemented`。

| P01-T01 | `TestP01T01MoneyParseBoundaries`、负数/溢出与算术边界 | `internal/domain/money.go`、`tests/contracts/money_contract_test.go` | `evidence/commands/phase01-contracts.txt` |
| P01-T02 | request/attempt/credential 合法与终态转移 | `internal/domain/state.go`、`tests/contracts/state_transition_contract_test.go` | `evidence/commands/phase01-contracts.txt` |
| P01-T03 | OpenAPI header/error/no-fake-success、validator、生成重复无 diff | `api/admin.openapi.yaml`、`api/admin_gen.go`、`api/generate.go` | `evidence/commands/phase01-openapi.txt`、`evidence/commands/phase01-generate-check.txt` |
| P01-T04 | 未知配置、production 开发开关、模板与显式环境 allowlist | `internal/config/`、`api/config.schema.json`、`configs/`、`tests/contracts/config_contract_test.go` | `evidence/commands/phase01-contracts.txt` |
| P01-T05 | JSON 未知/重复/大小写冲突/过深/多文档拒绝 | `internal/config/config.go`、`tests/contracts/json_ambiguity_contract_test.go` | `evidence/commands/phase01-contracts.txt` |
| P01-T06 | unsupported/unauthorized/unknown 分类与公共错误 JSON envelope | `internal/domain/error.go`、`internal/httpapi/errors.go`、`tests/contracts/error_envelope_contract_test.go` | `evidence/commands/phase01-contracts.txt` |
