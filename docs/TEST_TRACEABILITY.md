# 阶段 00 测试追踪

| test_id | 实际测试/检查 | 证据 |
|---|---|---|
| P00-T01 | `go test ./...`、`go vet ./...`、`go build -o urbino.exe ./cmd/urbino` | `evidence/raw/go-test.txt`、`go-vet.txt`、`go-build.txt` |
| P00-T02 | `go list -m all`；检查 `toolchain/dependencies.lock` 与 CPA/Sub2API reference revision | `evidence/raw/go-list-modules.txt` |
| P00-T03 | 配置样例静态检查；`internal/securitylog.TestLoggerDoesNotSerializeErrorText` | `configs/urbino.example.yaml`、`internal/securitylog/securitylog_test.go` |
| P00-T04 | `cmd/urbino` version/unknown/health 单测及真实 listener 检查 | `cmd/urbino/main_test.go`、`evidence/raw/cli-health.txt` |
