# evidence/tasks/02 · PersistenceTester

阶段 02 测试切片 · 2026-09-19 · 最终状态：done（测试入口由任务交付；最终断言由主 Agent在合并工作树补齐并实测）
最终源代码 revision：`working-tree:source-tree-sha256:11b9dd552a14c15a28d336e2847424c91f84710bd656402375ceaed692bc7c30`
## 交付

- `tests/integration/harness.go`：显式 `URBINO_TEST_DATABASE_DSN` 门禁，仅接受显式 loopback TCP、`*_test` 数据库；拒绝默认/生产目标、host/hostaddr/service/socket 路由覆盖；URL 与 keyword/value DSN 密码脱敏。
- `tests/integration/main_test.go`：无安全 DSN 或 `-short` 时输出 `NOT_RUN`；配置 DSN 但连接、构建或迁移失败时真实失败；迁移/serve 子进程只通过 0600 `database.dsn_file` 传递 DSN，不进入 argv 或 `URBINO_*` 子进程环境。
- `tests/integration/fixture_test.go`、`p02_t01_composite_fk_test.go`、`p02_t02_business_key_unique_test.go`、`p02_t03_concurrent_migrator_test.go`：P02-T01..T03 数据库约束、唯一性和并发迁移断言；主 Agent补充 T04..T07、价格/账本不变量、serve/history drift 和真实错误路径。

## 最终真实验证

命令：

```text
URBINO_TEST_DATABASE_DSN=<redacted-loopback-test-dsn> go test ./tests/integration -count=1 -v
```

结果：退出码 0；真实 PostgreSQL 16.15 临时实例；DSN 路由门禁、P02-T01..T07、已发布价格项 INSERT/UPDATE/DELETE、同事务发布、TEMP/DDL/账本权限、history drift 全部 PASS。原始输出：`evidence/commands/phase02-integration.txt`。

另有：

- `go test ./... -count=1 -v`：退出码 0；未配置真实 PG DSN 的集成项明确输出 `NOT_RUN`，非 PG 测试 PASS；输出：`evidence/commands/phase02-go-test.txt`。
- `go test ./internal/config ./internal/storage/postgres ./tests/integration -run 'Test(ReadDatabaseDSN|SafeErrorMessage|OpenRejectsBlankDSN|ResolveTestDSN)' -count=2 -v`：退出码 0；DSN 文件、错误脱敏和路由覆盖单测 PASS；当前用户无 chown 权限的 owner 负向子项按测试约定 SKIP；输出：`evidence/commands/phase02-unit.txt`。
- 临时 PostgreSQL 容器仅绑定回环地址，测试结束后已停止并删除；未读取或写入生产凭据。

## 测试映射

- P02-T01：`TestP02T01CompositeTenantForeignKeys`、`TestP02T01CrossTenantCompositeForeignKeyRejected`，requests、attempts、settlements、journal entries 的复合 tenant FK 越租户拒绝。
- P02-T02：`TestP02T02BusinessKeyUniqueness`、`TestP02T02BusinessKeysAreUnique`，request idempotency、attempt 业务键、settlement business key 唯一；同时验证空价格发布拒绝。
- P02-T03：`TestP02T03ConcurrentMigratorLock`、`TestP02T03ConcurrentMigratorsSerialize`，并发 migrator advisory lock 与单次 history 记录。
- P02-T04：`TestP02T04ApplicationRoleCannotDDLOrMutateLedger`，不平衡 journal 拒绝，应用角色无 DDL、journal/余额/结算/迁移历史危险写入权限。
- P02-T05：`TestP02T05CanceledTransactionRollsBack`，事务已写入后中途取消，完整回滚且 pool 可复用。
- P02-T06：`TestP02T06SchemaCompatibilityRejectsNewerVersion`、`TestP02T06MigrationHistoryDriftRejected`，migrate/serve future schema 与 history checksum drift 非零拒绝且 schema snapshot 不变。
- P02-T07：`TestP02T07DatabaseErrorsAreRedacted` 与 config/postgres 脱敏回归，真实 pgx/CLI 路径不回显 DSN/driver 内容。

## 子任务路由事实

首轮 `urbino-tester` 在测试体完成前按主 Agent收敛指令返回 blocked；该状态没有被伪装为 PASS。最终测试代码和合并后证据由主 Agent复核、修复并实际运行；阶段总状态仍由主 Agent唯一维护。