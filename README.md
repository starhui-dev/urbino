# Urbino · Codex 分阶段开发提示词包

版本：1.1 · 更新日期：2026-09-12 · 文档语言：中文

**正式项目名称只有 Urbino，不设中文名，不添加品牌后缀。** 纯后端，无前端，不依赖 CPA、CPA SDK、Sub2API 或其他成品中转站。仓库、二进制及镜像基础名称统一为 `urbino`，默认配置文件为 `urbino.yaml`，应用自有环境变量前缀为 `URBINO_`。

本次 1.1 更新统一命名、CLI/路径/配置/发布约定、进度元数据和命名检查；保留原有 20 阶段、安全边界与上线门禁。原有项目名称及旧实现均不继承。完整命名契约见 [范围与命名](docs/00-scope.md)。

这是供 Codex 在你的仓库中逐步实现的规格与提示词，不是已经编译、联调或通过审计的网关源码。包内的工具只负责组装提示词、检查文档和验收记录，不会自动部署或访问真实上游。

## 一、使用方式

1. 新建空仓库，把本包全部内容解压到仓库根目录。已有仓库先检查差异，不覆盖原有 AGENTS.md 或代码。
2. 在 Codex 中打开该仓库。首次发送下面的启动提示词，后续每次仅推进一个阶段。
3. 每阶段结束先发送下面的独立复核提示词；测试通过、证据齐全后才进入下一阶段。
4. 第 18 阶段进行真实上游和部署环境验证，第 19 阶段决定 GO / NO-GO。没有凭据、Docker、测试环境或授权证据时，记录阻塞项，不能伪造通过。

### 首次启动，直接粘贴

```text
请先阅读仓库根目录 AGENTS.md、MASTER_PROMPT.md、docs/00-scope.md、
docs/01-architecture.md 和 progress/state.json。
项目正式名称只有 Urbino，不设中文名；代码标识使用 urbino，环境变量前缀 URBINO_。
这是从零开发的纯后端 AI 中转站，不使用 CPA/S2A 的代码或 SDK。
现在仅执行 prompts/00-foundation.md，实际创建代码、运行检查并记录证据，
不要只输出计划，也不要提前实现其他阶段。遇到环境限制如实记录，
能在本阶段完成的代码与本地测试继续完成，不要把未运行的检查标成通过。
```

### 完成一个阶段后，另开会话复核

```text
阅读 AGENTS.md、MASTER_PROMPT.md 和 progress/state.json，
执行 prompts/REVIEW.md，复核最近一个 implemented 阶段。
从实际代码、测试和命令输出判断，不信任上一轮完成声明。
发现问题就修复并重跑相关检查；通过后才将该阶段标记 verified。
不要自行部署生产、调用真实付费上游或执行破坏性数据库操作。
```

### 继续开发

```text
阅读 AGENTS.md、MASTER_PROMPT.md 和 progress/state.json。
按 phases.json 找到第一个未 verified 的阶段；如果它是 implemented，先复核，
否则执行该阶段提示词。一次只完成一个阶段，保存代码、测试与可核实证据。
不要跳过前置条件，不要重复创建已经完成的模块。
```

中断续接使用 `prompts/RESUME.md`；阶段阻塞时使用 `prompts/REPAIR.md`。默认不自动 git commit/push，不使用绕过沙箱或审批的参数。

## 二、技术栈

Go 1.27.1 作为本包编制时核对的基线，chi v5、pgx v5、sqlc、goose、PostgreSQL 18.x、Valkey、valkey-go、River、OpenAPI / oapi-codegen、slog、Prometheus，OpenTelemetry 可选。所有精确依赖版本与镜像摘要由第 00 / 17 阶段实际核对、锁定，禁止 `latest`。

参考：Go [官方下载](https://go.dev/dl/)；[PostgreSQL 版本政策](https://www.postgresql.org/support/versioning/)；[chi](https://github.com/go-chi/chi)；[sqlc/pgx](https://docs.sqlc.dev/en/v1.31.1/guides/using-go-and-pgx.html)。详细来源见 `references/SOURCES.md`。

## 三、第一版上线范围

- 多租户、逻辑用户、项目、API Key、模型与上游管理；管理 API + CLI，不做注册网站或管理网页。
- OpenAI Chat Completions / Responses / Embeddings、Anthropic Messages / count_tokens、Gemini generateContent / streamGenerateContent / countTokens 的**已声明能力子集**；同协议优先，不承诺所有客户端全部功能。
- API Key 上游完整链路；通用 OAuth 授权、刷新、隔离与恢复机制完整实现。具体 OAuth 提供商只有在授权、协议和真实测试通过后才启用。
- 账号与组织配额、并发、冷却、会话归属、有限重试；用量、不可变账本、额度预留、审计调整。
- Docker Compose / 外部 PostgreSQL 与 Valkey 两种部署；安全配置、监控、备份恢复、滚动升级与回滚。

第一版不做在线支付、公开注册、WebSocket / Realtime、图像生成、音视频、Files / Batch、跨协议自动转换、动态插件、Kubernetes。遇到这些功能明确返回不支持，不能静默降级；未来增加须走 ADR、适配、计价、测试和上线门禁。

## 四、账号安全与授权边界

可借鉴 CPA/S2A 的凭据管理、刷新协调、会话归属、调度隔离和错误处理思路；**不能把实现了这些机制等同于不会封号**。不实现客户端身份伪装、验证码/挑战绕过、代理轮换逃避限制、重复使用已撤销令牌或撞库式登录。网络出口配置用于明确授权的路由与稳定性，不用于绕过地区或服务限制。

Anthropic 当前文档对第三方使用消费订阅凭据存在明确限制，因此本包默认不启用 Claude 消费订阅转发。OpenAI 及其他提供商的账户、使用限制和集成授权也需要按实际合同核对。详见 [Anthropic 文档](https://code.claude.com/docs/en/legal-and-compliance)、[OpenAI 服务协议](https://openai.com/policies/services-agreement/)。这不是对任意经营模式作出的法律合规保证。

## 五、目录

| 路径 | 用途 |
|---|---|
| `AGENTS.md` | 短而强约束的仓库规则 |
| `MASTER_PROMPT.md` | 分阶段执行与验收调度 |
| `phases.json` / `prompts/00-19` | 20 阶段依赖、任务和验收要求 |
| `docs/` | 架构、数据库、协议、安全、计费、管理与运维规格 |
| `references/` | 实际核对的官方链接、CPA/S2A 参考取舍 |
| `checklists/` | 测试用例、上线门禁与故障演练 |
| `templates/` | ADR、阶段证据、授权记录、上线报告模板 |
| `progress/` | 初始状态；所有阶段均未开始 |
| `tools/` | 无第三方依赖的 Python 提示词拼装和包完整性检查工具 |

可选：`python tools/compose_prompt.py 00 --out ./urbino-phase-00.md` 生成该阶段组合提示词；该相对路径同时适用于 POSIX shell 和 PowerShell。直接让 Codex 阅读仓库文件即可，不必使用拼装工具。

**重要：`python tools/validate_pack.py` 只验证提示词包结构与命名契约，不代表 Urbino 已通过程序测试或达到上线标准。**

包自身检查结果见 [PACK_VALIDATION.md](PACK_VALIDATION.md)。发行包逐文件摘要见 `MANIFEST.sha256`；它只用于检查原始提示词包完整性。
