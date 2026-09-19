# 阶段 01 · 契约字段策略

## 管理 API

管理 API 使用 `api/admin.openapi.yaml` 作为唯一版本化契约，标题固定为 `Urbino`。所有管理 mutation 要求 `Idempotency-Key`；并发更新要求 `If-Match`。分页 cursor 绑定过滤条件与作用域，limit 有上限。错误统一为 `code`、`message`、`request_id`、`retryable`、`details`，details 只允许安全诊断字段。

尚未启用的操作只能返回 `unsupported_capability`（HTTP 501）或其他明确错误，不得返回假成功的 2xx。资源不存在与跨租户资源统一为安全的 404。未知、影响权限/计费/外部请求/持久化的字段拒绝。

## Provider 字段

- **allowed**：协议规范明确、已有契约测试且不改变权限/计量语义的字段。
- **forbidden**：网关不执行的 hosted tools、私有客户端字段、任意 URL/代理/凭据字段、伪造身份或签名字段。
- **bounded-pass-through**：协议允许但暂不解释的安全扩展；必须保留有界字节数、键名/深度/数组长度限制，不得影响路由、授权、账务或出站地址。

跨协议转换默认不支持；不能通过丢弃未知字段或改写模型来伪装成功。

## 配置来源

文件选择保持 `--config > URBINO_CONFIG > ./urbino.yaml`，只选择一个文件，不合并。应用变量只接受显式声明的 `URBINO_HEALTH_ADDR`、`URBINO_LOG_LEVEL`、`URBINO_ENV`；不根据 YAML 字段自动猜测环境变量。生产环境禁止 `development: true`，秘密使用后续阶段定义的受限引用，不进入模板、日志或 CLI 参数。
