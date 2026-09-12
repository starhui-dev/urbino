# 阶段 02 独立复核

复核 revision：`2f5a9254a23037251b77cb244dbc2200fbdb0fa4`。

## 复核结果

- `migrations/001_initial.sql` 已建立租户复合外键、请求/尝试唯一性、价格版本与结算引用、账本租户与币种约束；价格和账本行有不可变触发器，延迟触发器要求分录至少两行且余额为零。
- `sql/queries.sql` 的 `InsertUsageEvent` 已写入必填 `tenant_id`，并经 sqlc 重新生成 `internal/storage/postgres/generated/queries.sql.go`。
- `postgres.Migrate` 使用显式 goose session lock；`CheckSchema`、DSN 拒绝/脱敏和 `RunTx` 取消感知退避均有实现。
- 新增真实 PostgreSQL 集成测试入口 `tests/pgtest/integration_test.go`，仅接受 `URBINO_TEST_DATABASE_URL` 且由 `tests/pgtest.ValidateDSN` 限制 loopback 隔离数据库。
- 本轮复核确认 `002_stage02_guards.sql` 按版本化方式补充租户币种复合外键、schema 版本 2 及价格发布后不可变；价格项修改按父版本 ID 顺序加行锁。
- `RunTx` panic、取消和回滚失败路径均有回归测试；回滚失败不会被取消上下文吞掉。

## 已执行检查

- `go test -count=1 ./...`：PASS，输出 `evidence/raw/review02-go-test.txt`。
- `go vet ./...`：PASS，输出 `evidence/raw/review02-go-vet.txt`。
- `go build -o %TEMP%\\urbino-stage02-review.exe ./cmd/urbino`：PASS，输出 `evidence/raw/review02-go-build.txt`。
- `go test -tags integration -run '^$' ./tests/pgtest`：PASS（仅编译），输出 `evidence/raw/review02-integration-compile.txt`。
- `git diff --check`：PASS，输出 `evidence/raw/review02-diff-check.txt`。
- 最终修复后 `go test -count=1 ./...`、`go vet ./...`、`go test -tags integration -run '^$' ./tests/pgtest`、`git diff --check`：PASS，输出 `evidence/raw/repair02-final4-*`。
- `go test -race ./...`：未完成；默认 CGO 关闭，启用 CGO 后本机没有 `gcc`（退出码 1，输出 `evidence/raw/review02-go-race.txt`）。阶段原有 race 证据属于修复前 revision，不能替代本次复核。

## 阻塞与未覆盖

- 本机没有 Docker、`psql` 或 PostgreSQL 服务（`evidence/raw/repair02-docker.txt`），也没有 `URBINO_TEST_DATABASE_URL` 隔离 DSN。因此集成测试未运行，P02-T01 至 P02-T06 仍不能判定通过：fresh/repeat/并行迁移、运行角色权限、真实跨租户 FK、取消回滚和 schema 升级兼容尚未取得运行时证据。
- `project.json` 与 `docs/14-naming.md` 不存在；命名继续依据 `AGENTS.md` 与 `docs/00-scope.md`。

结论：保持 `blocked`，不进入阶段 03；取得隔离 PostgreSQL 后运行 `go test -tags integration ./tests/pgtest` 并补齐证据，再进行下一次独立复核。

## 本次复核（2026-09-12 17:27 +08:00）

- 重新运行 `go test -count=1 ./...`、`go vet ./...`、`go build -o %TEMP%\\urbino-stage02-current.exe ./cmd/urbino`、`go test -tags integration -run '^$' ./tests/pgtest` 与 `git diff --check`，均通过；原始输出见 `evidence/raw/repair02-current-*`。
- 运行 `go test -tags integration ./tests/pgtest`（未设置 `URBINO_TEST_DATABASE_URL`）按设计对 5 个集成用例明确失败，未静默跳过；输出见 `evidence/raw/repair02-current-integration-no-dsn.txt`。
- `docker` 与 `psql` 命令均不可用；没有发现可运行 PostgreSQL，因此 P02-T01 至 P02-T06 仍无真实运行时证据。
- 当前环境 `go test -race ./...` 因 CGO 未启用退出码 2；未将其记为通过。`tools/check_evidence.py 02` 按设计因阶段为 blocked、存在 blocker 及 P02-T01 至 P02-T06 未运行而退出码 1。
- 复查 `internal/storage/postgres`、`migrations` 与 `tests/pgtest` 未发现可在当前环境修复的实现或测试回归；保持阶段状态 `blocked`。
