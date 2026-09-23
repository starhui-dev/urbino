# 阶段 03 身份边界测试映射

本文件只记录实际测试结果，不把 SKIP 记为 PASS。测试使用合成 ID/秘密；证据输出不包含完整秘密。

| 测试 | 结果 | 证据 | 说明 |
|---|---|---|---|
| P03-T01 | PASS | `evidence/commands/phase03-identity-full.txt` | public key 与 admin listener 隔离；admin token 不接受 public path；前缀和 public principal admin scope 隔离。 |
| P03-T02 | PASS | `evidence/commands/phase03-scoped-store.txt` | 真实 loopback PostgreSQL tenant scope 单项读取、列表过滤、统计和缺失/foreign lookup 通过。 |
| P03-T03 | PASS | `evidence/commands/phase03-scoped-store.txt` | 真实 loopback PostgreSQL project membership 与跨项目/tenant boundary 通过。 |
| P03-T04 | PASS | `evidence/commands/phase03-identity-full.txt` | TTL 上界、共享后端撤销传播、缓存错误 fail-closed、版本不一致均通过。 |
| P03-T05 | PASS | `evidence/commands/phase03-identity-full.txt` | 过期、温缓存截止点、scope 缺失和固定 bootstrap admin scope 通过。 |
| P03-T06 | PASS | `evidence/commands/phase03-identity-full.txt`、`evidence/commands/phase03-persistent-auth-bootstrap.txt` | 并发 bootstrap 单胜者、旧秘密不重置、O_EXCL 独占输出以及持久化认证通过。 |
| P03-T07 | PASS | `evidence/commands/phase03-identity-full.txt` | 重复/冲突 header、query key、畸形 Authorization、listener 方向和规范单头通过。 |
| P03-T08 | PASS | `evidence/commands/phase03-identity-full.txt` | digest-only record、错误/日志脱敏、admin token 一次性通过。 |
| P03-T09 | PASS | `evidence/commands/phase03-identity-full.txt` | 多模型策略、空策略拒绝、同用户多 key 策略隔离通过。 |

稳定性：`go test -tags identityimpl ./tests/identity -count=2 -v` 实测退出码 0；原始输出见 `evidence/commands/phase03-identity-full.txt`。真实 loopback PostgreSQL 管理/认证集成 `go test -tags identityimpl ./tests/identity ./internal/storage/postgres ./internal/httpapi -count=1` 实测退出码 0；原始输出见 `evidence/commands/phase03-persistent-auth-bootstrap.txt`。

未验证：未验证生产 Valkey/River/provider、部署网络、备份恢复和升级回滚；这些不属于本阶段已验证能力。