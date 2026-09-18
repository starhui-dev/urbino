# 来源与核对范围

本版保留旧包技术参考记录（旧包记录日期 2026-09-12），**不声称在 2026-09-18 重新核验所有版本、协议或条款**。R19 改为本次实际读取的 OMP 官方原生发现实现；其他 29 项沿用历史来源。新的 OMP 详细依据见 [OMP-SOURCES.md](OMP-SOURCES.md)。

未完成参考仓库全量安全审计、运行 CPA/S2A、账号风控实测或真实 OMP 调用。读过源码不等于已验证其运行属性。仓库 `main` 是动态分支，本次未取得固定 commit SHA；实施时记录实际版本/revision、日期、读取文件与 ADR，不能虚构。

本包架构、默认阈值、测试门禁和错误语义是独立设计选择，不是参考源码复制品。提供商条款、授权、配额和接口必须在启用前再核实。

| ID | 来源 | 用途/限制 | 记录日期 |
|---|---|---|---|
| R01 | [CPA 官方仓库](https://github.com/router-for-me/CLIProxyAPI) | 项目边界与文档入口；未作全面审计 | 2026-09-12 |
| R02 | [CPA 配置样例](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml) | 管理访问、重试/冷却配置；不复制 fingerprint/cloaking 实现 | 2026-09-12 |
| R03 | [CPA auth conductor](https://github.com/router-for-me/CLIProxyAPI/blob/main/sdk/cliproxy/auth/conductor.go) | 调度组件接口参考；不嵌入 SDK | 2026-09-12 |
| R04 | [CPA LICENSE](https://github.com/router-for-me/CLIProxyAPI/blob/main/LICENSE) | 编制时根文件为 MIT；具体复用另行审查 | 2026-09-12 |
| R05 | [Sub2API 官方仓库](https://github.com/Wei-Shaw/sub2api) | 项目及官方自述入口；不将 README 当安全认证 | 2026-09-12 |
| R06 | [Sub2API Edge Security](https://github.com/Wei-Shaw/sub2api/blob/main/deploy/EDGE_SECURITY.md) | 入口、可信代理、SSE 与反代边界 | 2026-09-12 |
| R07 | [Sub2API Token Refresh](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/token_refresh_service.go) | 刷新协调、失效与版本保护的参考 | 2026-09-12 |
| R08 | [Sub2API Concurrency](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/concurrency_service.go) | 并发租约与取消；不继承 fail-open 默认 | 2026-09-12 |
| R09 | [Sub2API Account Scheduler](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/openai_account_scheduler.go) | 强资源归属、软亲和与能力过滤参考 | 2026-09-12 |
| R10 | [Sub2API LICENSE](https://github.com/Wei-Shaw/sub2api/blob/main/LICENSE) | 编制时根文件为 LGPL v3，非复制许可结论 | 2026-09-12 |
| R11 | [OAuth 2.0 Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html) | PKCE、token 限权、刷新轮换和重放风险 | 2026-09-12 |
| R12 | [OWASP SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html) | URL、域名/IP、DNS 与重定向边界 | 2026-09-12 |
| R13 | [Anthropic Legal and Compliance](https://code.claude.com/docs/en/legal-and-compliance) | 第三方集成和消费订阅凭据限制，启用前重核 | 2026-09-12 |
| R14 | [OpenAI Services Agreement](https://openai.com/policies/services-agreement/) | 账户/凭据/使用限制；不是对所有经营模式的授权 | 2026-09-12 |
| R15 | [OpenAI Rate Limits](https://developers.openai.com/api/docs/guides/rate-limits) | 限流及配额处理依据 | 2026-09-12 |
| R16 | [OpenAI Streaming Responses](https://developers.openai.com/api/docs/guides/streaming-responses) | SSE 和响应事件协议 | 2026-09-12 |
| R17 | [Anthropic Streaming](https://platform.claude.com/docs/en/build-with-claude/streaming) | 原生事件、终止与用量语义 | 2026-09-12 |
| R18 | [Gemini Tokens](https://ai.google.dev/gemini-api/docs/tokens) | countTokens 与 usageMetadata | 2026-09-12 |
| R19 | [OMP 原生资源发现](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/discovery/builtin.ts) | 原生项目 context/agents/skills 发现；对应 raw 源码已读，不等于本机已加载 | 2026-09-18 |
| R20 | [Go Releases](https://go.dev/dl/) | Go 版本官方入口；本次未重核具体补丁，实施前核对锁定 | 2026-09-12 |
| R21 | [Go net/http](https://pkg.go.dev/net/http) | Transport、context、连接与响应控制 | 2026-09-12 |
| R22 | [Go crypto/cipher](https://pkg.go.dev/crypto/cipher) | 标准 AEAD 接口 | 2026-09-12 |
| R23 | [chi](https://github.com/go-chi/chi) | 标准 net/http 路由 | 2026-09-12 |
| R24 | [sqlc + pgx](https://docs.sqlc.dev/en/v1.31.1/guides/using-go-and-pgx.html) | 类型化 SQL 与 pgx 集成 | 2026-09-12 |
| R25 | [goose](https://github.com/pressly/goose) | 迁移工具 | 2026-09-12 |
| R26 | [PostgreSQL Versioning](https://www.postgresql.org/support/versioning/) | 18 系列支持状态与补丁原则 | 2026-09-12 |
| R27 | [Valkey Security](https://valkey.io/topics/security/) | 网络隔离与认证 | 2026-09-12 |
| R28 | [valkey-go](https://github.com/valkey-io/valkey-go) | Go 客户端 | 2026-09-12 |
| R29 | [River Transactional Enqueue](https://riverqueue.com/docs/transactional-enqueueing) | PG 事务入队 | 2026-09-12 |
| R30 | [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) | OpenAPI 类型和客户端/服务端生成 | 2026-09-12 |

原始记录保存在 [sources.json](sources.json)；其中 verification_status 区分历史继承与本次源文件阅读。不能把参考链接存在或语法校验成功当成授权/联调/上线通过。
