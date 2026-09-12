# 03 · 威胁模型与账号风险控制

## 信任边界

外部客户端、HTTP 头、JSON 字段、模型输出、上游错误正文和第三方仓库都不可信。即使管理员配置的 upstream URL 也必须校验，因为管理员令牌泄漏或误配置不能直接变成访问云元数据的通道。

需要保护：上游 API Key / OAuth token、管理令牌、用户访问令牌、租户隔离、余额、会话/响应 ID、代理凭据、审计完整性、可用性与对话隐私。

## 风险 -> 强制防护 -> 验证

| 风险 | 实现要求 | 至少一个负向验证 |
|---|---|---|
| 管理接口暴露 | 独立 listener，默认回环，独立凭据；远程管理要求 TLS/mTLS 或受控私网策略 | public key 和公网 :8080 不能访问管理路由 |
| 用户 Key 泄漏 | 仅显示一次；摘要验证、吊销、最小 scopes、到期；URL 不接受 key | list API / 错误 / 日志找不到 key |
| 上游秘密泄漏 | AEAD、密钥分离、密钥文件受限；默认无正文日志 | dump、trace、panic、代理 URL 均不能漏秘 |
| 跨租户 | 每个资源引用都校验 tenant/project/principal | 篡改 user/project/account/response id 在出站前失败 |
| SSRF | 统一出站，域名白名单、地址验证、DNS 绑定、禁止重定向 | metadata、IPv6 映射、DNS 重绑定、代理远程 DNS 测试 |
| 配额绕过 | quota_group 按真实组织/主体聚合；账号重复导入不复制额度 | 同组织两把 key 共用限制 |
| 刷新竞态 | DB claim + credential_version + nonce；结果未知暂停 | 两实例只能发一次刷新；旧结果不能覆写新凭据 |
| 重试风暴 | 统一尝试预算、reset/Retry-After、禁止输出后重试 | 连续 429/403 不轮换扫完整账号池 |
| 请求耗尽 | header/body/JSON 深度/事件/并发/队列/连接有界 | 慢客户端、巨大事件、恶意无限流可终止 |
| 账目破坏 | 不可变账本、幂等、短事务、未知状态 | 并发扣款、worker 重放、杀进程不重复结算 |
| 日志数据外泄 | 默认不记录 prompt/tool args/response/token；结构化安全字段 | 模拟 secret 被注入错误正文后不出现在日志 |
| 软件供应链 | 依赖锁定、扫描、SBOM、镜像签名/摘要 | 关键依赖漏洞或来源不明阻止发布 |

## “降低封号风险”的可实现范围

本项目控制自己的行为：不超越已授予权限、尊重实际配额、减少错误请求、避免 refresh token 重复消费、避免跨租户会话串用、支持明确稳定的出口、收到停用/挑战后暂停和告警。

本项目不控制提供商的政策、账户审查或误判；不声称模拟某种 TLS/UA 特征就能避免封号。现有开源项目中出现的 fingerprint/cloaking 功能不是本项目的验收目标。

遇到 403 challenge/suspension 或授权撤销：隔离相关账户/组织范围并记录原因，要求管理员依据提供商流程处理。不能轮换账号、代理或伪造客户端使请求继续穿过同一限制。对普通限流也须遵守其 reset 边界，不能把同主体另一 key 当作新额度。

## 安全默认配置

生产 TLS 验证必须开启；管理默认 localhost；可信代理默认空集合；上游 HTTPS-only；公共 provider 默认禁止私网/保留地址；错误正文最多读取受限字节且不原样记录；CORS 默认不开放；metrics/pprof 不暴露公网。

访问日志默认记录 request_id、匿名主体引用、模型目录 ID、状态、耗时和用量完整性。审计仅记录动作/资源/操作者及经过白名单的前后元数据，不记录凭据值或原始请求。

Secret 处理：开发者必须知道 Go 的内存管理不能保证所有字符串副本可靠清零，不可作此承诺。尽量减少复制、短期作用域、受限读取；进程/core dump/调试权限由部署防护。启动不得输出完整配置、环境变量或 DSN。

## 许可证与参考代码

本包以原则和测试场景参考 CPA/S2A，独立实现，不携带其源码。编制时 CPA 根许可证为 MIT，S2A 根 LICENSE 为 LGPL v3；这是文件观察，不是对任意复制/静态链接方式的法律结论。第 00 阶段仍需锁定具体 commit 并检查文件级许可证。不得通过“改变量名”把复制代码说成独立实现。

参考：[CPA 配置](https://github.com/router-for-me/CLIProxyAPI/blob/main/config.example.yaml)、[S2A 入口安全](https://github.com/Wei-Shaw/sub2api/blob/main/deploy/EDGE_SECURITY.md)、[OWASP SSRF](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)。上述项目不是经过本包全面审计的安全背书。
