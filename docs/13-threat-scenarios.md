# 13 · 必做事故演练

每次只在自有隔离环境演练，禁止对真实提供商做压力、恶意刷新或账户封禁测试。

## A. 两实例同时刷新

准备会轮换 token 的 mock OAuth server；两个独立进程同时触发到期刷新。期望只有一个 refresh 请求，数据库新版本唯一，其余进程使用新版本。再在 token server 已消费旧 token、网关保存前 SIGKILL；重启后进入 uncertain，不再发送旧 token。管理员换 key 与旧 refresh 返回并发也必须测试。

## B. 多租户响应 ID 猜测

租户 A 发 Responses 得到 ID；租户 B 携相同 ID，改 project/user/key 组合重试。期望上游请求计数不增加，所有入口/查询接口都拒绝；同租户不同无权项目也一样。缓存清空/多实例重启后仍成立。

## C. 连续 429 和组织配额

同 quota_group 两账号、独立 group 第三账号。mock 返回 429 与 Retry-After；同组后续调度均受限，软亲和不绕过；只有策略允许、授权独立且属于普通可用性 fallback 时才能考虑第三账号。模拟 403 suspension 则不尝试别的身份继续完成受限请求。

## D. 用户断流

收到部分事件后关闭客户端；断开前/后分别返回 usage。记录取消延迟、上游连接释放、lease 释放、usage known/unknown 与 hold 状态。不能将 canceled 一律记成免费，也不能读完模型再偷偷完成。

## E. PG 失败与结算

分别在 reservation 前、dispatch 后、usage 收到后、ledger commit 前/后中断 PG。保证新请求闭锁、已有请求不重复调上游、预留/账本可核对、重放 worker 不重复收费。终态缺证据时进入 pending/unknown，不凭猜测改为成功。

## F. Valkey 丢状态

持续长流时 kill/restart Valkey 或在专用测试库模拟丢状态；所有进程必须停新 admission，处理未确认租约，epoch 恢复完成前不能从 0 计数接满流量。分别测连接失败、主切换、时间漂移与 process pause。记录可控制的限制边界，不能用一次正常网络测试宣称强一致。

## G. 出站代理/SSRF

由测试 DNS 返回安全后再改私网；测试不同 A/AAAA、HTTP redirect、CONNECT 目标解析、代理故障、evil.example 后缀、169.254/IPv6 metadata、私有例外 profile。验证请求在发包前拦截、原始上游 key 不到达非允许地址、代理断开不直连。

## H. 金额/重复事件

100 个并发请求竞争只能支付 10 个请求的余额；所有已发出请求应已有合法 hold，拒绝的不触达上游。重复终态/usage/worker/adjustment web request，结算业务键唯一、journal 平衡；更改价格版本后旧请求不变。

## I. 恢复与升级

隔离恢复 PG+密钥材料；运行 ledger verify、查询租户、验证吊销与实际解密；删除旧 key 之前验证备份兼容。滚动升级在 SSE 中进行，readiness/drain/版本兼容正确；测试应用 rollback，不执行不可逆 down 冒充恢复。

## J. 秘密和高基数

把合成密钥放入上游错误正文、proxy URL、panic 信息、工具参数、超长模型名；检查日志/metrics/traces/报告不泄漏，标签基数受限，日志量有界。开启诊断也不能输出真实环境变量或会话。
