# 05 · 安全出站、代理与网络边界

## 统一 TransportFactory

所有推理、OAuth、健康检查和可选 webhook/telemetry 的外部请求必须通过统一策略层。禁止适配器自行 `http.Get`、`http.DefaultClient`、`ProxyFromEnvironment` 或创建无安全策略的新 client。单测需要静态扫描和运行时探针验证不能绕过。

连接池 key 至少包含 provider origin、credential/account 隔离范围、egress_profile/version、TLS policy。请求 Header 每次新建并注入认证，不能并发复用可变 Header 或 Request。OAuth/cookie 状态不跨账户共享；第一版默认不使用 cookie jar。

## URL/DNS/SSRF

上游 origin 由管理员持久配置且须审核，不接受客户端请求中的 base_url、proxy_url、account_id 或任意目标 URL。

解析并规范化 scheme/host/port/path；拒绝 userinfo、fragment、控制字符、反斜线混淆和无法规范化的 IDN。HTTPS-only；默认禁用 redirect。允许的 provider path 用固定构建器，不允许用户路径逃出 base path。

公网 profile：阻断 loopback、私网、link-local、云元数据、CGNAT、multicast、unspecified、文档/保留范围及 IPv4-mapped IPv6 的等价地址。DNS 的 A/AAAA 全部验证；混合不安全结果默认拒绝。通过受控 DialContext 连接本次已验证 IP，TLS ServerName 仍是允许的主机名，不能验证一次域名后由默认 Dialer 再解析一次。

连接池重用和 DNS 策略变更：policy/egress 版本变化淘汰旧连接；新连接重新验证。测试覆盖 TTL 改变、DNS rebinding、CNAME、IPv6、重定向和域名后缀伪装。

明确的私有模型服务是例外 profile：需要独立 allowlist、指定 CIDR、允许端口和审计记录；不是全局 allow_private=true。dev mock 的 loopback 例外仅在测试/开发模式可启用，production 配置不接受。

## 代理

支持管理员配置 HTTP CONNECT / HTTPS 代理与本地解析的 SOCKS5；代理密码从 vault 取得。禁止 client 请求切换代理。

**本机校验 DNS 并不能自动保护代理侧重新解析。** 对 CONNECT / SOCKS5 要么把已验证目标 IP 交给代理并保留正确 TLS SNI，要么在受控出口代理上强制同等目标策略并提供测试证据。不能做到则该代理模式不能生产启用。SOCKS5H/其他远程 DNS 模式第一版不启用。

egress 失效默认 fail closed，不偷偷直连。修改 egress 是受审计配置变更：新请求用新版本，旧请求自然结束或受控取消，不把某次 403 当自动换 IP 的理由。不会承诺固定 IP 本身可防封号。

## 头部

移除下游 Authorization、Proxy-Authorization、Cookie、Host 覆盖、连接专用头，以及 Connection 指定的扩展 hop-by-hop 头。按协议显式注入服务端凭据和必要版本头。

不盲传 `x-*`。tenant/project/组织身份和计费字段由已授权配置产生。User-Agent 使用真实的 `urbino/<version>`；确有协议协商需求的官方版本头由 provider contract 定义，不伪造设备标识、TLS/JA3/JA4 或官方客户端签名。

响应只透传允许的内容类型、必要协议头和安全 request-id/rate limit 信息；Set-Cookie、内部代理信息、真实上游凭据和未经检查的错误正文不外泄。

入口的 X-Forwarded-For 只在直接对端属于 trusted_proxies 时解析，按链从右到左剥离可信跳；空列表表示不信任转发头。Nginx/CDN 负责覆盖用户可伪造头，源站禁止公网直连。ACL 不用不可信请求头决定。

## 时间与资源限制（设计默认，不是实测容量）

header 读取 10 秒，header 64 KiB；管理请求体 1 MiB；文本推理体默认 8 MiB、最大可配置 32 MiB；JSON 深度 64；upstream error body 64 KiB；单 SSE event 1 MiB，超过明确终止。

TCP connect 10 秒、TLS 10 秒、response-header 60 秒、流空闲 120 秒、总请求最长 1800 秒；按 provider 可调但有系统上限。流式 http.Client.Timeout 不设全局短值；通过 context、读空闲 watchdog 和每次下游写 deadline 管理。管理 listener 可用 30 秒总期限。

流式 listener 的全局 WriteTimeout=0 不是无限放行慢客户端：必须有 per-write deadline 和队列上限。压缩事件流默认关闭，反代 buffering 关闭；客户端取消立即传播 context，不继续后台读完整生成。最终记账可用独立有界 cleanup context，但不能借它延长推理执行。

参考：[Go net/http](https://pkg.go.dev/net/http)、[OWASP SSRF](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)、[S2A 入口安全说明](https://github.com/Wei-Shaw/sub2api/blob/main/deploy/EDGE_SECURITY.md)。
