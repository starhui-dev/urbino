# 来源与核对范围

核对日期：2026-09-12。以下为本包编制时实际打开的官方仓库、源文件、官方文档和标准。仓库代码通过对应 raw 页面阅读；此处使用 GitHub 可读页面链接。

**未完成全仓库安全审计、运行 CPA/S2A、复现其安全属性或实测账号风控。** 参考代码主分支未固定 commit，第 00 阶段必须补充真实 revision。代码存在某功能不代表它有效、合规或适合直接采用。

本包详细架构、默认阈值、测试门禁和故障语义均为独立设计选择，不是对参考项目的逐段复制。第三方提供商文档和条款可能变化，正式启用前再次核对。

| ID | 真实来源 | 本次用途 |
|---|---|---|
| R01 | [CPA 官方仓库](https://github.com/router-for-me/CLIProxyAPI) | 项目边界与文档入口；未作全面审计 |
| R02 | [CPA 配置样例](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml) | 管理访问、重试/冷却配置；不复制 fingerprint/cloaking 实现 |
| R03 | [CPA auth conductor](https://github.com/router-for-me/CLIProxyAPI/blob/main/sdk/cliproxy/auth/conductor.go) | 调度组件接口参考；不嵌入 SDK |
| R04 | [CPA LICENSE](https://github.com/router-for-me/CLIProxyAPI/blob/main/LICENSE) | 编制时根文件为 MIT；具体复用另行审查 |
| R05 | [Sub2API 官方仓库](https://github.com/Wei-Shaw/sub2api) | 项目及官方自述入口；不将 README 当安全认证 |
| R06 | [Sub2API Edge Security](https://github.com/Wei-Shaw/sub2api/blob/main/deploy/EDGE_SECURITY.md) | 入口、可信代理、SSE 与反代边界 |
| R07 | [Sub2API Token Refresh](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/token_refresh_service.go) | 刷新协调、失效与版本保护的参考 |
| R08 | [Sub2API Concurrency](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/concurrency_service.go) | 并发租约与取消；不继承 fail-open 默认 |
| R09 | [Sub2API Account Scheduler](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/openai_account_scheduler.go) | 强资源归属、软亲和与能力过滤参考 |
| R10 | [Sub2API LICENSE](https://github.com/Wei-Shaw/sub2api/blob/main/LICENSE) | 编制时根文件为 LGPL v3，非复制许可结论 |
| R11 | [OAuth 2.0 Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html) | PKCE、token 限权、刷新轮换和重放风险 |
| R12 | [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html) | URL、域名/IP、DNS 与重定向边界 |
| R13 | [Anthropic Legal and Compliance](https://code.claude.com/docs/en/legal-and-compliance) | 第三方集成和消费订阅凭据限制，启用前重核 |
| R14 | [OpenAI Services Agreement](https://openai.com/policies/services-agreement/) | 账户/凭据/使用限制；不是对所有经营模式的授权 |
| R15 | [OpenAI Rate Limits](https://developers.openai.com/api/docs/guides/rate-limits) | 限流及配额处理依据 |
| R16 | [OpenAI Streaming Responses](https://developers.openai.com/api/docs/guides/streaming-responses) | SSE 和响应事件协议 |
| R17 | [Anthropic Streaming](https://platform.claude.com/docs/en/build-with-claude/streaming) | 原生事件、终止与用量语义 |
| R18 | [Gemini Tokens](https://ai.google.dev/gemini-api/docs/tokens) | countTokens 与 usageMetadata |
| R19 | [Codex AGENTS.md 官方指南](https://developers.openai.com/codex/guides/agents-md) | 仓库指令机制；指南当前可重定向到官方 Learn 页面 |
| R20 | [Go Releases](https://go.dev/dl/) | 编制时核对 Go 1.27.1；实施时固定补丁版本 |
| R21 | [Go net/http](https://pkg.go.dev/net/http) | Transport、context、连接与响应控制 |
| R22 | [Go crypto/cipher](https://pkg.go.dev/crypto/cipher) | 标准 AEAD 接口 |
| R23 | [chi](https://github.com/go-chi/chi) | 标准 net/http 路由 |
| R24 | [sqlc + pgx](https://docs.sqlc.dev/en/v1.31.1/guides/using-go-and-pgx.html) | 类型化 SQL 与 pgx 集成 |
| R25 | [goose](https://github.com/pressly/goose) | 迁移工具 |
| R26 | [PostgreSQL Versioning](https://www.postgresql.org/support/versioning/) | 18 系列支持状态与补丁原则 |
| R27 | [Valkey Security](https://valkey.io/topics/security/) | 网络隔离与认证 |
| R28 | [valkey-go](https://github.com/valkey-io/valkey-go) | Go 客户端 |
| R29 | [River Transactional Enqueue](https://riverqueue.com/docs/transactional-enqueueing) | PG 事务入队 |
| R30 | [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) | OpenAPI 类型和客户端/服务端生成 |

## 固定版本记录的格式

后续在 dependency/reference lock 中保存 repo、实际 commit、访问日期、使用文件、许可证观察和设计 ADR。未读取的源码不能写“已验证”。无网络时保留明确 BLOCKED，不填假 SHA。
