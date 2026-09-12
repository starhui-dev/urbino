# 10 · 配置、运行与运维

## 配置契约

默认运行配置文件 `urbino.yaml`，配置路径按 `--config` > `URBINO_CONFIG` > 当前工作目录 `urbino.yaml` 选择；显式指定的文件缺失必须报错，不静默回退。应用自有环境变量只使用 `URBINO_` 前缀并逐项声明，不能任意导出全部环境变量。

第 01 阶段定义 schema，第 17 阶段实际生成并验证 `configs/urbino.example.yaml`、`configs/urbino.production.example.yaml`、`.env.example` 和 JSON Schema。不能文档使用一个字段名、代码读取另一个。未知字段、非法时长、空 secret 引用、无上限的资源值应拒绝启动。

启动配置节：server(public/admin/internal)、database、valkey、secrets、transport、security、limits、billing、jobs、observability、retention。只允许持久配置 provider endpoints；客户请求不能覆写。

secret 使用 `_file`/KMS 引用，真实秘密不出现在示例；环境变量优先级明确且仅列出允许项。不要自动继承 HTTP_PROXY/HTTPS_PROXY。日志只输出配置摘要和版本，secret/DSN 一律脱敏。

生产配置验证必须检查：管理不在公网；provider allowlist 非空；TLS 检验开启；秘密可读且权限正确；PG/Valkey 内网与认证；账本模式及币种；schema 兼容；所有生产能力已通过授权与 live gate；没有测试开关。

## 健康端点

liveness 只表示进程未死锁，不因短暂 PG 故障引发重启风暴；readiness 包含 schema、PG、Valkey epoch、账务闸门、是否 draining。指标端点需要受控网络和可选专用鉴权；pprof 默认关闭，仅显式内网临时开启。

## 可观测性

slog JSON；固定服务标识 `urbino`；自有 Prometheus 指标前缀 `urbino_`；OpenTelemetry `service.name=urbino`，worker 使用角色属性区分。标准库/第三方指标保留原名称。request_id 串联 request/attempt，字段白名单。指标至少覆盖 admission/rejection、active/waiting、upstream latency/TTFT、stream abort、quota/cooldown、refresh success/uncertain、billing pending、jobs lag、db pool、memory/goroutines。

metric labels 只能是有限枚举/provider 类型/已知模型目录 ID 等受控值；禁止 request_id、用户、Key、完整 URL、原始错误、任意客户端 model 作为 label。每请求具体信息放受限日志/数据库，不把 Prometheus 变成日志库。

trace 禁止采集 Authorization、Cookie、prompt、完整响应和 tool args；SDK 自动 instrumentation 也要检查，不能以“库默认安全”跳过。

告警规则：任何 secrets 泄漏测试失败；refresh uncertain；账本不平；unknown usage/hold 异常累积；403/suspension 激增（暂停并人工处理）；PG/Valkey admission 闭锁；错误率与 p95/TTFT；证书/secret 到期；磁盘/备份失败。

## 后台任务

River 与 PG 事务结合（按 ADR 选直接 transactional enqueue 或 outbox-dispatch）；job payload 只含资源 ID/版本，不含 token/正文。job handler 按 at-least-once 防重复，最大次数、backoff、dead-letter/失败可见性和人工处理完整。

任务：usage 聚合、账本核对、未终结 request/hold 检查、过期流程清理、授权到期检查、审计外发（可选）、有预算的健康检查。不是不断向模型发送收费 prompt。refresh 采用 04 文档的单次消费状态机，不能用通用 job 无限重试刷新。

## 保留策略（设计默认，需运营方确认）

prompt/response 默认不存。请求元数据 30 天、聚合用量 180 天、OAuth flow 到期后 24 小时内清理、软 affinity 24 小时；审计和账本的保存策略独立，不能因为上述 TTL 删除。法律或合同要求的期限由运营方配置，不把这些工程默认值当法定标准。

清理前检查强引用和账务未结状态；日志自动轮转、磁盘配额。导出仅元数据/用量，鉴权、时间范围和分页有界。

## 优雅停机

先 readiness=false -> 停止新 admission -> 停 Worker 新取任务 -> 等待有界 drain（默认 60 秒）-> 主动取消剩余流并记录 incomplete/unknown -> 有界 finalize -> 释放本机资源 -> 退出。

数据库事务不随已取消的请求 context 永久无法结算；cleanup context 独立但时间有界。进程被 SIGKILL 不执行清理，依赖持久 attempt 和恢复闸门处理。平台 termination grace 要大于应用 drain+cleanup。

参考：[River 事务任务](https://riverqueue.com/docs/transactional-enqueueing)、[Valkey 安全](https://valkey.io/topics/security/)。
