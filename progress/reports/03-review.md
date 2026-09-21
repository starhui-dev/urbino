# 阶段 03 · 独立复核报告

结论：BLOCKED；不能标记 verified，需进入后续修复动作。

## 复核上下文

- 代码复核：`Stage03CodeReview-2`，角色 `urbino-reviewer`，新上下文，只读，未运行命令、未修改文件。
- 安全复核：`Stage03SecurityReview-2`，角色 `urbino-security`，新上下文，只读，未运行命令、未修改文件。
- 实际路由：`Hui AI (OpenAI)/gpt-5.6-sol:xhigh`；路由与 `evidence/omp/runtime.json` 一致。
- reviewed revision：`working-tree:source-tree-sha256:29630cda7c87790e41877eade7d0711699934c70b350d924698be0fff682223d`。
- 两个 reviewer 均给出 `BLOCKED`；主 Agent 未把 reviewer 摘要当作实测证据。

## 阻塞问题

1. **P1：最终测试证据不可审计且不能绑定 revision。** `evidence/commands/phase03-identity-full.txt` 与 `phase03-scoped-store.txt` 仅有人工摘要，没有原始 transcript、测试计数、数据库生命周期或 revision。可读的旧长 transcript `phase03-identity-linked.txt` 仍显示 `ErrPGAdapterPending` 和 T02/T03 SKIP。现有摘要不足以证明最终工作树上 P03-T01..T09 的完整 PostgreSQL/运行链测试确实执行。
2. **P1：P03 全套测试主要绑定进程内 Service，不是生产 PostgreSQL authenticator。** `tests/identity/binding_identityimpl.go` 的 T01、T04-T09 仍调用 `internal/auth.Service` 的 keys/admins/principals map；只有 T02/T03 使用 PostgreSQL ScopedStore。没有可审计的真实 public/admin verifier 重启、多实例、持久化撤销/过期测试。
3. **P1：持久化 public key 的非 active 状态未在实际认证路径硬拒绝。** `internal/storage/postgres/authenticator.go` 的 `lookupPublic` 未限定 `k.status = 'active'`，`auth.Verify` 也不检查 `KeyRecord.Status`；迁移允许 `expired` 状态而不要求 `expires_at` 已到期。未来到期前被标记 `expired` 的 key 可能通过真实 listener。
4. **P1：PostgreSQL bootstrap 关键路径未被 P03-T06 实测覆盖。** P03-T06 调用内存 `auth.Service.BootstrapAdmin`，不是 `postgres.BootstrapAdmin`；runtime 摘要没有原始输出、请求响应、并发结果或 revision。文件 O_EXCL 输出先于数据库提交，还存在 DB 提交失败留下孤立输出文件的交叉故障窗口，未见恢复协议。
5. **P1：认证失败限流可被并发和源端口轮换绕过。** `CredentialMiddleware` 将 `Check` 与失败后的 `RecordFailure` 分开，并发请求可同时通过检查；限流键直接使用 `RemoteAddr`（通常含源端口），新连接轮换端口即可创建新桶。现有测试没有并发失败入口或端口轮换场景。
6. **P1：真实 listener 证据覆盖不足。** runtime 摘要只证明 health=200、admin token 在 admin listener=200、同 token 在 public listener=401、admin 无凭据=401；没有证明有效持久化 public key 通过、PG 撤销/过期、runtime header/query 冲突、认证回调失败和限流行为。
7. **P2：admin token 的未来 `revoked_at` 未在 runtime 路径显式拒绝。** `lookupAdmin` 与 `VerifyAdmin` 允许未来撤销时间窗口；当前阶段没有持久化 admin token revoke 测试。该项属于剩余 fail-closed 风险。

## 已确认的代码观察

- `internal/httpapi/auth.go` 已强制调用 listener 对应的认证回调并将 principal 写入请求上下文。
- `internal/cli/cli.go` 在显式配置 database、public/admin listener 和 pepper 文件时装配 `postgres.Authenticator` 与独立 listener。
- public/admin 前缀、header/query 冲突、HMAC-SHA-256 digest、恒定时间比较、复合 tenant/project/member 外键、active-member trigger、PostgreSQL advisory lock/O_EXCL 代码路径可读到。
- 这些代码观察不能替代缺失的最终运行证据，也不能覆盖上述可达缺陷。

## 证据与范围判断

- reviewer 未运行测试、构建或命令；本报告只记录其对代码与仓库证据的观察。
- `go test ./...` 的阶段证据包含 database-gated skips；这不等于阶段三集成通过。
- 生产 PostgreSQL、Valkey 多实例、River、真实 provider、完整管理资源 API、部署、备份恢复和升级回滚仍是阶段范围外/后续验证项，不因本次复核单独阻塞。
- 旧 `phase03-identity-linked.txt` 必须明确标为修复前证据，不能与最终 PASS 摘要混用。

## 结论与下一步

当前阶段保持未验证并标记 `blocked`。下一动作应是单独的修复阶段：先修正 persistent authenticator 的 status/fail-closed 约束与失败限流 key/原子预算，再补真实 PostgreSQL/runtime 测试及可审计原始输出，之后针对最终 revision 重新执行 `prompts/REVIEW.md`。本轮不修改生产代码、不自动进入阶段四、不提交或部署。
