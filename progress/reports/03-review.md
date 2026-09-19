# 阶段 03 · 独立复核报告

结论：BLOCKED；不能标记 implemented 或 verified。

复核上下文：`Stage03CodeReview`（urbino-reviewer）与 `Stage03SecurityReview`（urbino-security），独立只读上下文；未修改文件、未运行命令。复核对象为阶段三最终工作树及主 Agent 已保存证据。

## 阻塞问题

1. **P0：认证中间件此前存在形状认证绕过。** `internal/httpapi/auth.go` 原先只检查 credential kind/prefix，未验证 HMAC、过期、撤销、auth_version 或 principal。主 Agent 已将接口改为强制调用 `AuthenticatePublic`/`AuthenticateAdmin` 回调，并补了认证服务失败测试；但当前仍未找到 public/admin listener 的真实装配调用，因此必须重新取证，不能把中间件单测当网络认证证明。
2. **P0：实际 HTTP surface 仍只有 health listener。** `serve` 未装配 public/admin listener、管理 API 或 public data plane。
3. **P0：P03-T02/T03 的 ScopedStore 仍返回 `ErrPGAdapterPending`。** 7 项 tenant/project persistence 测试是显式 SKIP，不能计为 PASS。
4. **P0：API key project membership 双边界此前缺失。** 主 Agent 已在根迁移和内置迁移中加入 `(tenant_id, project_id, user_id)` 对 `project_members` 的外键、active-member trigger，并在 `CreateAPIKey` 同一事务内检查 active member；该修复尚未在真实 PostgreSQL 上重新实跑。
5. **P0：Service 凭据、CLI bootstrap 与 PostgreSQL/Valkey 未形成持久化运行链。** Service 的 key/admin records 仍是进程内 map；CLI bootstrap 写 DB 后没有运行时 verifier 从 DB 读取；重启和多实例路径未验证。
6. **P0：PostgreSQL bootstrap 文件与事务交叉故障未做真实 smoke。** `O_EXCL` 局部语义有测试，但文件创建与 DB commit 不是可证明的原子操作。
7. **P1：失败限流原先会消耗合法请求配额。** 主 Agent 已增加 `Check`/`RecordFailure`，中间件改为只记录认证失败；需要后续装配测试证明实际行为。
8. **P1：admin token scope、token persistence/revoke 和 scoped repository API 仍不完整。** 主 Agent已限制内存 token scope 不得超过 principal，并将认证返回 token scopes；数据库 token verifier/revoke 和完整 scoped store 仍未接入。

## 证据判断

- linked identity seam：P03-T01、T04-T09 有 PASS；P03-T02/T03 为 SKIP。
- `go test ./...`、`go vet ./...`、`go build ./cmd/urbino`、生成检查和 CLI help 已执行并记录；新 middleware、membership migration 修复后应重新运行并替换 evidence revision。
- P03-T01 的 socket listener isolation、P03-T06 的 PostgreSQL bootstrap、P03-T02/T03 的真实 PostgreSQL boundary、进程重启/多实例持久认证均 NOT_RUN。
- 测试/证据不得把 in-process seam PASS 描述为端到端 PASS；当前阶段状态应为 `blocked`。

## 建议

先完成真实 listener/auth-service/PG/Valkey 装配和 ScopedStore，再运行显式 loopback PostgreSQL 阶段三集成测试，更新 test matrix、traceability、stage evidence，最后重新执行 reviewer 与 security reviewer。