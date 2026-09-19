# evidence/tasks/02 · MainIntegration

阶段 02 主 Agent 整合 · 2026-09-19
最终源代码 revision：`working-tree:source-tree-sha256:11b9dd552a14c15a28d336e2847424c91f84710bd656402375ceaed692bc7c30`

## 实际命令与结果

- `go generate ./api ./internal/storage/postgres && make generate-check`：退出码 0；OpenAPI、sqlc 生成重复运行无差异，根目录与内置 migration SQL 一致；`evidence/commands/phase02-generate.txt`。
- `go vet ./...`：退出码 0；`evidence/commands/phase02-go-vet.txt`。
- `go build ./cmd/urbino`：退出码 0；`evidence/commands/phase02-go-build.txt`。
- `go test ./... -count=1 -v`：退出码 0；无显式 PG DSN 的集成项按门禁记录 NOT_RUN；`evidence/commands/phase02-go-test.txt`。
- `go test ./internal/storage/migrate -count=1 -v`：退出码 0；迁移版本缺口和非法 SQL 文件名拒绝；`evidence/commands/phase02-migrate-unit.txt`。
- `jq empty api/config.schema.json && go run github.com/getkin/kin-openapi/cmd/validate@v0.142.0 api/admin.openapi.yaml`：退出码 0；`evidence/commands/phase02-schema.txt`。
- `go run ./cmd/urbino migrate --help`：退出码 0；`evidence/commands/phase02-cli-help.txt`。
- `go test ./internal/config ./internal/storage/postgres ./tests/integration -run 'Test(ReadDatabaseDSN|SafeErrorMessage|OpenRejectsBlankDSN|ResolveTestDSN)' -count=2 -v`：退出码 0；DSN 文件安全、错误脱敏与测试目标门禁 PASS；owner 负向子项因当前用户无 chown 权限按测试约定 SKIP；`evidence/commands/phase02-unit.txt`。
- `URBINO_TEST_DATABASE_DSN=<redacted-loopback-test-dsn> go test ./tests/integration -count=1 -v`：退出码 0；PostgreSQL 16.15 临时 loopback 实例；P02-T01..T07、价格项 INSERT/UPDATE/DELETE 保护、同事务发布、TEMP/DDL/账本权限、历史漂移和 DSN 路由门禁 PASS；`evidence/commands/phase02-integration.txt`。

## 整合修复

- 迁移 SQL 增加 price_items、NUMERIC 精确单价、price snapshot、租户币种复合 FK、发布价格项 INSERT/UPDATE/DELETE 保护、发布价格项延迟约束和 journal 平衡延迟约束。
- 迁移历史增加 SQL SHA-256 checksum、连续前缀/name/checksum 漂移校验和稳定 future-version 错误；应用角色夹具改为最小权限并逐项验证危险写操作拒绝，包括 PUBLIC/app TEMP 撤销。
- DSN 文件通过 no-follow fd/fstat 读取；集成 DSN 门禁拒绝 host/hostaddr/service/socket 路由覆盖。
- 迁移资源内置到二进制；`serve` 与 `migrate` 不依赖当前 CWD，显式 database 配置的 serve 只做 compatibility check，不自动迁移。
- P02 测试增加全部复合 FK 越租户负向路径、余额/结算/迁移历史权限、账本不平衡/混币种、已发布价格项 INSERT/UPDATE/DELETE、事务中途取消、serve compatibility、history drift 和真实 pgx/CLI 脱敏路径。

## 未验证边界

未运行生产 PostgreSQL、Valkey、真实 provider、部署、备份恢复、升级回滚或管理 API；临时 PostgreSQL 仅用于合成集成数据并已停止删除。阶段状态只写 `implemented`，等待新上下文独立 reviewer。