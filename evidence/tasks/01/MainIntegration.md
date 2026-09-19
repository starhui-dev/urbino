# 阶段 01 · 主 Agent 整合证据

- 日期：2026-09-18
- 范围：修复契约测试发现的两个生产缺口并在最终工作树重跑阶段检查。

## 修复

1. `internal/domain.PublicError` 增加 snake_case JSON tags；`domain.UUID` 实现 `encoding.TextMarshaler`，确保 `request_id` 是规范 UUID 字符串。
2. `internal/config.StrictJSONDecode` 对 JSON object key 做大小写归一化去重，拒绝 `health_addr` 与 `Health_Addr` 这类会被 `encoding/json` 视为同一目标字段的冲突。

## 实测命令

| 命令 | 退出码 | 结果 |
|---|---:|---|
| `gofmt -w internal/domain internal/config` | 0 | pass |
| `go test ./tests/contracts -count=2 -v` | 0 | 46 个顶层结果（23 个测试函数重复两次）全部通过 |
| `go test ./...` | 0 | 全部包通过 |
| `go vet ./...` | 0 | 无诊断 |
| `go build ./cmd/urbino` | 0 | 构建成功 |
| `make generate-check` | 0 | oapi-codegen v2.8.0 第二次生成无 diff |
| `jq empty api/config.schema.json` | 0 | JSON Schema 语法通过 |
| `go run github.com/getkin/kin-openapi/cmd/validate@v0.142.0 api/admin.openapi.yaml` | 0 | OpenAPI 校验通过 |

原始命令输出分别保存于 `evidence/commands/phase01-contracts.txt`、`phase01-go-test.txt`、`phase01-go-vet.txt`、`phase01-go-build.txt`、`phase01-generate-check.txt`、`phase01-openapi.txt`。

## 任务状态

- `Stage01ContractTests` 初次测试真实退出码 1，证据保留在 `evidence/tasks/01/ContractsTester.md`；失败断言未删除。
- 主 Agent 根据该证据修复后重跑，最终契约测试退出码 0。
- 原 `urbino-implementer` 与备用 `task` 实际路由均返回 `deepseek-v4-flash` 不支持（404），未产生代码；实现由主 Agent在批准文件范围内完成。该路由异常不伪装为实现任务 PASS。
