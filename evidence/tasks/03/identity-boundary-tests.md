# 阶段 03 身份边界测试映射

本文件只记录实际测试结果，不把 SKIP 记为 PASS。测试使用合成 ID/秘密；证据输出不包含完整秘密。

| 测试 | 结果 | 证据 | 说明 |
|---|---|---|---|
| P03-T01 | PASS | `evidence/commands/phase03-identity-linked.txt` | public key 与 admin listener 隔离；admin token 不接受 public path；前缀和 public principal admin scope 隔离。 |
| P03-T02 | SKIP | `evidence/commands/phase03-identity-linked.txt` | 4 项 tenant scope 测试因 PostgreSQL ScopedStore adapter 未绑定而显式 SKIP；不是通过。 |
| P03-T03 | SKIP | `evidence/commands/phase03-identity-linked.txt` | 3 项 project membership/boundary 测试因同一 adapter 未绑定而显式 SKIP；不是通过。 |
| P03-T04 | PASS | `evidence/commands/phase03-identity-linked.txt` | TTL 上界、共享后端撤销传播、缓存错误 fail-closed、版本不一致均通过。 |
| P03-T05 | PASS | `evidence/commands/phase03-identity-linked.txt` | 过期、温缓存截止点、scope 缺失和 admin scope 提升拒绝通过。 |
| P03-T06 | PASS | `evidence/commands/phase03-identity-linked.txt` | 并发 bootstrap 单胜者、旧秘密不重置、O_EXCL 独占输出通过。 |
| P03-T07 | PASS | `evidence/commands/phase03-identity-linked.txt` | 重复/冲突 header、query key、畸形 Authorization、listener 方向和规范单头通过。 |
| P03-T08 | PASS | `evidence/commands/phase03-identity-linked.txt` | digest-only record、错误/日志脱敏、admin token 一次性通过。 |
| P03-T09 | PASS | `evidence/commands/phase03-identity-linked.txt` | 多模型策略、空策略拒绝、同用户多 key 策略隔离通过。 |

稳定性：`go test -tags identityimpl ./tests/identity -count=2 -v` 实测 56 PASS / 14 SKIP / 0 FAIL，退出码 0；原始输出见 `evidence/tasks/03/run-tagged-count2.txt`。默认构建 `go test ./tests/identity -count=1` 退出码 0，但身份实现与 ScopedStore 均按构建约定显式 SKIP；原始摘要见 `evidence/tasks/03/run-default-build.txt`。

未验证：P03-T02/T03 需要显式 `URBINO_TEST_POSTGRES_DSN` 或约定 loopback PostgreSQL 实例，并绑定真实 PostgreSQL ScopedStore；当前不能宣称 tenant/project persistence boundary 已 PASS。
