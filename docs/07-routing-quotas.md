# 07 · 调度、会话、限流、并发与重试

## 选择上游的顺序

先过滤授权用途、租户权限、协议与模型能力、account 状态、egress 状态、quota_group、预算；再处理强制资源归属；最后才使用权重和负载。

策略默认“优先级 + 同优先级加权最少在途请求”，权重有界，结果可解释。价格变化、模型别名和 provider 切换要显式策略，禁止用更便宜/更弱模型静默替代。

## 强归属与软亲和

`previous_response_id` / provider resource ID 是强归属：tenant/project/principal/provider/account 全部匹配才发送。未知或不属于当前调用方的 ID 在出站前拒绝；归属账户不可用时不能改另一个账号碰运气。

软 session affinity 只优化无状态请求，key 为 HMAC(tenant,project,principal,session_id,protocol)，避免不同租户同字符串碰撞。软亲和不能覆盖权限、冷却、额度、能力或安全停用。客户端不能指定真实 account ID。

缓存命中不是跨账户或跨租户共享许可。第一版不缓存模型响应，不共享 prompt/tool/result 内容。

## 配额范围

至少支持 tenant/project/key/account/实际组织 quota_group 的 RPM、TPM 预算和并发；一个实际主体的不同 key/import record 必须共享其提供商限制。提供商文档说明的 model/organization/reset window 必须入范围模型，不默认每 key 独立。

本地预算估算不是提供商剩余额度真值。usage 与 reset headers 是带时间的观测；失效标为 unknown，不显示伪造剩余值。TPM 预占与最终修正仍不能保证服务端 tokenizer/窗口完全一致，配置需保守且尊重上游 429。

## 并发租约

Valkey 使用唯一 lease_id、owner epoch、过期时间与原子 Lua 申请/续租/释放；释放必须核对 ownership，重复释放无副作用。租户/项目/key/账号/组织的限制要么一次原子检查取得（单 Valkey 实例），要么使用固定顺序获取+失败补偿；不能部分成功后泄漏配额。

第一版不使用 Valkey Cluster，多 key Lua 不假设跨 slot 原子。真实部署模式要写入 manifest。

默认 lease 30 秒、每 5 秒续租，续租无法确认时在租约失效前取消本地推理，不能持有无效租约继续运行。等待队列有总长度/租户长度/最长 2 秒期限，取消立即退出。默认初始账户并发 1 是保守设计起点，不是对任何平台“安全并发”的事实判断。

### 重启/故障恢复

Valkey 丢状态、failover、flush、epoch 不一致：暂停新 admission，进入恢复闸门。利用 PG 中活跃 attempt/epoch、硬执行期限和持久安全停用恢复/等待；不得直接从空计数器重新接满并发。恢复操作需有所有实例一致的 epoch；不能仅检测到 PING 成功就恢复。

网络取消不一定能停止提供商已启动的计算；租约只能约束网关自身可控制的请求。故障测试必须量化残余请求和冷却窗口，不承诺任意分区下绝对无超额。

## 状态与分类

account：active / cooling / requires_reauth / quarantined / disabled。状态原因结构化；credential_expired 与 account_suspended 不同；model_unsupported 只影响能力，不自动封整个账号。

| 事件 | 动作 | 默认重试 |
|---|---|---|
| 请求不合法、未支持能力 | 客户端错误，不降低账号健康 | 否 |
| API key 401 | credential invalid，隔离并告警 | 否 |
| OAuth 文档确认 token 过期 | 协调刷新一次；确认请求未执行才可再试 | 严格受总尝试数限制 |
| 403 权限/组织政策/挑战/封禁 | 按错误范围拒绝或 quarantine；不换身份逃避 | 否 |
| 429 明确 rate limit | 记录范围与 reset/Retry-After，排队上限内等待否则返回 | 不立即换同范围 key |
| 额度耗尽/付款问题 | 标记额度不可用；管理员处理 | 否 |
| 连接前 DNS/TLS/TCP 失败 | 可确认未发送时记录安全失败 | 最多总 2 次 |
| 5xx / 请求写后超时 | 结果可能未知；仅 provider 明确保证未受理或幂等才可重试 | 默认否 |
| 已向客户端输出 | 发送协议错误/终止、结算已知用量 | 绝不重试 |
| 客户端取消 | 立即取消上游，记录取消与 usage 完整性 | 否 |

不能只看 status 就推断已执行/未执行。`httptrace.WroteRequest` 可以辅助取证，不是提供商执行证明。首次 SSE 注释/HTTP header 发出前的重试策略也必须明确，不能用心跳隐藏已提交响应。

Retry-After 支持秒/HTTP-date，进行格式与范围验证；服务端明确较长 reset 不能被本地上限截短后提前重试，改为标记禁用到期或人工处理。退避抖动只用于散开已允许的重试，不模拟真人。

只有明确获准且配额独立的可用上游，才可在普通可用性故障下按配置 fallback。不能在安全拒绝、策略限制或同组织额度耗尽时进行规避性 fallback。

参考：[CPA 调度配置](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)、[S2A scheduler](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/openai_account_scheduler.go)、[S2A concurrency](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/concurrency_service.go)、[OpenAI 限流文档](https://developers.openai.com/api/docs/guides/rate-limits)。默认参数均为本项目设计，不照抄参考值。
