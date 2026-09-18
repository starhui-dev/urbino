# 01 · 模块化单体架构

## 运行拓扑

客户端 -> TLS 反向代理 -> 数据面 :8080 -> 应用编排 -> 安全 Transport -> 授权上游。
管理 CLI -> 管理面 :9090 -> 应用服务。
应用/Worker -> PostgreSQL；应用 -> Valkey。指标/内部健康 :9091。

开发时一个二进制可同时运行 API、管理和 Worker；生产可用同一镜像分进程 `urbino serve` / `urbino worker`。无内部 RPC。初期不自动选主网关节点，所有关键共享状态显式处理。

## 包边界（按阶段实际创建）

```text
cmd/urbino/
internal/domain/             # 值对象、状态、错误；不依赖 SQL/HTTP 框架
internal/app/                # admission、dispatch、finalize、management
internal/auth/               # 下游及管理鉴权
internal/vault/              # 上游秘密加解密与受限读取
internal/oauth/              # 授权流程、刷新与版本控制
internal/transport/          # DNS/SSRF/代理/TLS/头部/连接池
internal/protocol/           # public codecs、SSE、安全解析
internal/provider/           # openai/anthropic/gemini/authorized-oauth
internal/routing/            # 能力过滤、亲和、冷却与重试判定
internal/quota/              # 原子限流、并发租约、恢复闸门
internal/metering/           # provider usage -> typed usage
internal/billing/            # 价格快照、额度预留、账本结算
internal/storage/postgres/   # sqlc 查询与仓储
internal/storage/valkey/     # Lua/缓存/租约
internal/jobs/               # River handlers
internal/httpapi/            # public/admin/internal listeners
internal/cli/                # 调管理 API 的薄命令层
internal/observability/
api/ migrations/ sql/ configs/ deploy/ tests/
```

handler 不直接更新余额/凭据；provider 不能调用默认 http.Client 越过安全边界；只有 vault 返回短生命期的认证材料，handler/模型对象不携带秘密。

## 一次请求的执行顺序

1. 可信客户端 IP、入口资源限制、request_id；不记录正文。
2. 下游凭据验证 -> tenant/project/user -> 模型与协议授权。
3. 有界解析和能力校验；确定请求预算、路由和配置版本。
4. 创建 request 行和幂等占位；进行用户/项目 admission。候选筛选和配额快查不等于授权完成。
5. 选定上游与价格；获取有界并发租约；短事务预留余额和请求预算并记录 attempt。任何后续失败都释放已取得资源。
6. 最后重查停用/凭据版本/组织冷却；取得认证材料；发起上游 HTTP。不得在网络期间持有数据库事务或行锁。
7. 逐段转发。响应 ID 归属绑定在向客户端暴露该 ID 之前持久化。收集只包含计量字段的 usage。
8. 完成时短事务写 usage、结算、状态及 outbox/任务；然后发出协议的最终成功事件。流式中途失败走协议错误/断开路径，不能再改 HTTP 状态。
9. 释放租约；后台处理汇总、审计分发、保留策略和未终结请求检查。

对于非流式响应，只读取配置允许的有界大小；完成结算后才输出成功。对于流式响应，已发送部分内容不等于最终成功，也不能保证上游计费随客户端取消立即停止。

## 配置来源与快照

启动文件：监听、数据库/Valkey 连接引用、密钥路径、TLS/可信代理、最大资源限制。
数据库：租户、模型、价格、provider、账号、授权与路由。
Valkey：可重建缓存/短期调度状态。

每次请求固定 policy_version / route_version / price_version / credential_version；热更新只影响之后请求。安全停用和密钥撤销是例外，必须可在 dispatch 前阻断。

## 默认故障策略

PG 不可用：拒绝新计费请求与管理写；已有流尽力受控结束，无法持久化终态则不发成功终止事件。Valkey 不可用：拒绝新推理；不能降级为每实例无限并发。Worker 不可用：计费主路径不丢账，告警队列积压；超过配置预算拒绝新工作。

本设计不宣称分布式租约在任意网络分区下仍绝对线性一致，也不把数据库事务推广成上游执行 exactly-once。具体恢复要求见 07 / 09 / 12 文档。
