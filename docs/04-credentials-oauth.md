# 04 · 密钥、OAuth 与刷新生命周期

## 三套凭据不得混用

下游 API Key：高熵随机秘密（至少 32 随机字节），public_id 用于查找，摘要用于恒定时间验证；示例外形 `gw_live_<id>.<secret>`。摘要用独立 HMAC-SHA-256 pepper；随机 key 不是人类密码，不用慢密码 KDF 替代必需的入口限流。支持 pepper 版本轮换与双读过渡。

管理令牌：独立前缀与表、scope、过期策略；只能在管理 listener 使用。

上游凭据：API Key、access/refresh token、代理密码属于需取回的秘密，必须 AEAD 加密；禁止把它们混进账号的普通 JSONB 元数据。

## AEAD 存储格式

使用标准库 AES-256-GCM 或经审查的等效 AEAD；密钥来自受限文件/KMS 引用，不入库。格式包括 format_version、key_id、nonce、ciphertext；AAD 明确绑定 tenant/account/provider/auth_mode/credential_version。AAD 使用固定二进制长度前缀或稳定序列化，不能依赖 map 遍历顺序。

禁止重用 nonce；密钥轮换批量重加密要有 checkpoint、CAS 和中断恢复。轮换不得直接删除旧密钥，必须验证所有在线数据及仍保留的备份已具备解密路径。credential_version 改变时同步使用新 AAD 重加密，不要只改数字导致无法解密。

密钥文件权限检查必须适配运行系统；Linux 生产拒绝过宽权限，Windows 开发不能仅凭 Unix mode 声称 ACL 安全。密钥指纹使用另一把 HMAC key，不能把 token hash 当日志标识随意暴露。

## 管理初始化

`urbino bootstrap --output <path>` 仅在无管理员的全新数据库运行；用数据库唯一约束/锁防止双 bootstrap。输出文件必须独占创建，不覆盖、无默认口令、不在普通日志打印。重复执行不能重置已有管理员。

管理 CLI 从受限凭据文件或 stdin 获取秘密，不接受明文密钥命令行参数。list/get credential 只返回 ID、状态、到期、版本和掩码，不能解密导出。重新配置采用替换/轮换接口。

## 通用 OAuth 框架

仅实现公开且被允许的 OAuth 集成。每个 provider 的 grant、issuer、client_id、redirect URI、scope、audience 和 token endpoint 都来自核对的正式文档及自己获准使用的客户端配置；不借用某官方客户端的 client_id 来冒充该客户端。

Authorization Code：PKCE S256；state 高熵、存摘要、10 分钟默认有效；绑定当前 admin、tenant、provider、redirect URI 和流程 ID；回调一次性 CAS claim。管理员在自己的 CLI/浏览器完成授权，网关不托管登录 UI。OAuth callback 仅返回最小 JSON/文本状态，不构建前端。

CLI-only 可由管理员将授权结果提交到有鉴权的 exchange 接口；即便收到 URL 也只解析明确允许的 code/state，不按用户传来的 callback URL 发网络请求。

OIDC 若用于确认主体，必须验证签名、issuer、audience、nonce 和有效期；不能“解码 JWT 后相信 email”。普通 OAuth 不需要就不解析 ID token。设备授权仅在提供商公开支持时实现，尊重 polling interval / slow_down / expires_in。

禁止隐式授权流、密码授权、采集浏览器 Cookie 和未经允许的 session token 导入。通用 OAuth 测试使用自有 mock 授权服务器；消费订阅流不因 mock 测试通过就可生产启用。

## 刷新状态机

`idle -> claimed -> request_sent -> saved`，失败可能进入 `retryable_not_sent`、`uncertain`、`requires_reauth` 或 `disabled`。

1. 本进程 singleflight 仅减少重复；数据库 claim 是跨实例协调的事实来源。
2. 短事务验证 expires_at / credential_version / account 状态，CAS claim 并保存 attempt_nonce / deadline。
3. 在事务外请求 token endpoint；使用固定的获准出口配置，有严格超时。
4. 成功后短事务验证原 version + nonce + 当前管理版本，原子保存新 access/refresh token；上游未返回新 refresh token 时按该 provider 的明确文档决定保留旧值，不能写空覆盖。
5. 发布版本失效事件后其他节点重读。Pub/Sub 丢消息不能永久使用旧 token；有限 TTL 与版本检查兜底。
6. 失效/吊销/invalid_grant：暂停相关账户，不无限重试。

### 特别重要的崩溃窗口

服务端已经轮换 refresh token，但响应丢失/本地写库失败时，旧 token 是否还能用是不确定的。**不能在 claim 超时后简单重发旧 refresh token。** 到期的 request_sent claim 必须转 uncertain；只有该 provider 正式支持的恢复机制或重新授权可以解除。

fencing token 只能防止旧结果覆盖数据库，不能撤销已经发到提供商的刷新。因此不能声称单靠 Valkey TTL 锁解决了全部竞态。

管理员在刷新期间更换凭据：新 version 生效，旧响应只能被丢弃并安全清理；旧错误不能把新凭据标为失效。测试必须覆盖此竞态。

参考：[OAuth 安全 BCP RFC 9700](https://www.rfc-editor.org/rfc/rfc9700.html)、[S2A 刷新服务](https://github.com/Wei-Shaw/sub2api/blob/main/backend/internal/service/token_refresh_service.go)、[Go AEAD 文档](https://pkg.go.dev/crypto/cipher)。本节并非这些来源的逐字实现。
