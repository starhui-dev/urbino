# 00 · 范围、默认决策与不可变约束

本文是本项目的设计决策，不是对 CPA/S2A 原设计的复述，也不是用户已逐项确认的历史需求。

## 明确要求与本包默认

明确要求：项目正式名称为 Urbino；不设中文名；从零开发；纯后端；无前端；不用 CPA；参考 CPA/S2A 安全架构；由本包确定实现技术栈；Codex 分阶段交付。

为避免设计停滞，默认采用：一个运营方管理多个租户；每租户包含用户、项目、模型权限与余额；管理员创建租户和密钥；无公开注册、无密码登录、无邮件短信、无在线支付；余额通过带审计的调整接口维护。用户端只有程序调用和只读用量接口。后续接入支付/统一认证必须新增阶段，不假装已经实现。

## 固定命名契约

正式项目名称只有 **Urbino**；不另取中文名、不添加品牌后缀，不再把产品名当作占位符。中文文档中的“网关”“中转站”只是功能描述，不是中文名称。

| 对象 | 固定约定 |
|---|---|
| 对外项目名称、文档产品名、OpenAPI `info.title` | `Urbino` |
| 仓库名、二进制名、镜像基础名称 | `urbino` |
| Go 命令入口 | `cmd/urbino/` |
| Go module 的仓库部分 | `urbino`；未提供真实托管路径时使用 `example.com/urbino` |
| 默认运行配置文件 | `urbino.yaml`；显式 `--config` 可选其他路径 |
| 配置样例 | `configs/urbino.example.yaml`、`configs/urbino.production.example.yaml` |
| 应用自有环境变量 | `URBINO_` 前缀，如 `URBINO_CONFIG`；仅接受契约声明的变量 |
| Compose 应用/独立任务服务 | `urbino` / `urbino-worker`；执行同一个 `urbino` 二进制 |
| 出站 User-Agent | `urbino/<version>`，使用真实版本，不伪装其他客户端 |
| 应用自有 Prometheus 指标 | `urbino_` 前缀；第三方标准指标不强制改名 |
| OpenTelemetry 服务名 | `service.name=urbino`；worker 用独立角色属性区分 |
| 应用自有 Valkey 键空间 | `urbino:<deployment-id>:` 前缀；其后保留 tenant/project 等隔离维度 |
| 本提示词包发行文件名 | `urbino-codex-pack.zip` |

`example.com` 只是 Go module 托管域名的占位，不是项目名称；从当前仓库已明确的真实 remote 确认 owner/host 后统一替换 import path。镜像仓库示例可写 `ghcr.io/<owner>/urbino:<version>`，其中 owner/version 必须在实际发布前落实，不假定仓库、组织、商标、域名已经存在或属于用户。未获授权不创建远程仓库、不注册域名、不推送镜像。

最终需提供 `urbino serve`、`urbino worker`、`urbino migrate`、`urbino bootstrap`、`urbino version`、`urbino config validate`、`urbino health`、`urbino admin`、`urbino doctor`、`urbino ledger verify`。这些命令按对应阶段实际实现，不在第 00 阶段用假成功填满。

品牌更名不能修改第三方协议字段、提供商错误码或来源 URL；不能把 `GOOS`、`GOARCH`、`CGO_ENABLED`、`OTEL_*` 等标准变量机械加上应用前缀。所有开发文档、示例与测试都需要遵守同一契约。

## 已选栈

Go 1.27.1 基线；net/http + chi v5；PostgreSQL 18.x；pgx v5 + sqlc；goose SQL migration；Valkey + valkey-go；River PostgreSQL 任务；OpenAPI + oapi-codegen；slog + Prometheus；可选 OpenTelemetry；Docker Compose。第 00 阶段核对所有具体版本、兼容性、维护状态和许可证；对本包编制后新增的安全更新用 ADR 记录。不开启 Go 自动取任意 toolchain 的生产构建。

不使用 ORM、不引入 Kafka/NATS/ClickHouse/Elasticsearch、不拆微服务、不做动态 Go 插件。不需要完整用户身份平台，逻辑用户与随机访问令牌已满足第一版管理。

## 最小生产产品

必须实现 tenant/project/user/API Key、RBAC、upstream/provider/credential/quota group、模型目录与路由；请求/尝试/usage/ledger；管理 API/CLI；限流和冷却；加密、审计、观测、迁移、部署、还原。

三类原生协议适配均需编译与 mock 契约测试通过，但只有真实联调和授权通过的 provider 可以在生产启用。第一版上线至少要有一个真实上游完整通过，不允许全 mock 发布。

## 兼容性边界

支持范围详见 08-protocols.md。默认仅 text + client-side function tools + 已验证结构化输出；不在网关执行工具。图片输入可在不计费的受控测试模式验证，尚无完整用量上界和计价时不得开放到 prepaid 租户。

第一版不含 WS/Realtime、官方私有客户端接口、内部压缩接口、后台 Responses、媒体生成、文件/批任务、跨协议翻译、自动注册/验证码或订阅凭据采集。对未支持的 API/字段给出明确稳定错误，不能偷偷改模型、丢工具或用 Chat Completions 假装 Responses。

## 经营与授权模式

API Key 本身不是转售许可。每个实际启用上游的用途、数据处理范围、用户范围及许可由运营者核实，保存引用和审核记录。消费订阅 OAuth 默认禁用；通用 OAuth 仅服务有公开协议且允许该集成方式的提供商。技术实现不保证不会被停用、审查或限制。

## 验收优先级

P0：密钥泄漏、跨租户访问、重复扣款/信用越限、SSRF、凭据刷新竞态、无限重试、资源耗尽、结果未知被伪装为成功。
P1：配置回滚、真实兼容、日志观测、备份恢复、性能与容量。
P2：可选协议/提供商扩展、更多报表。P2 不得以一个返回成功的 stub 出现在发布能力清单。
