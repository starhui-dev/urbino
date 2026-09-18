# 09 · 用量、预留与不可变账本

## 四个不同实体

request：用户调用一次。attempt：向上游尝试一次。usage_event：某 attempt 的计量观测。settlement：按价格/用户规则形成的一次收费决定。它们不是一张“请求日志”里几个随时可改的字段。

上游成本与用户收费分开。安全重试导致上游产生的多次成本可以全部记录；用户默认只对一个确定的服务结果结算，不把重试乘数藏进收费。人工追加收费必须显式审计，不重写原记录。

## 计量模型

每个字段有 unknown/known 语义：缺字段不等于 0。至少包含 input_total、input_uncached、cache_read、cache_write（细分受 provider 支持）、output_total、reasoning、其他已支持维度；另有 usage_source、completeness、is_estimate、provider_version 和 rate tier。

OpenAI cached/reasoning 细分经常是父计数的子集；Anthropic cache read/write 具有自己的计量语义；Gemini thoughts/candidates/total 字段需按版本解释。不能把这些数字直接全部加总。各适配器给出字段映射表和算例 golden tests；所有等式有来源和适用条件。

原始 usage 只允许保存计量白名单字段，不保存完整回复或上游可能夹带秘密的任意对象。重复事件通过唯一 event_key/digest 合并；累计值采用最后有效快照或规范增量规则，不能重复累加。

## 金额和价格

账本最小单位为 10^-6 个配置币种；整数 BIGINT 检查溢出。价格表 NUMERIC 精确表示“每 1M token 的币种金额”；乘除在 decimal/big.Rat 或可靠 decimal 库中完成，最后每请求统一舍入到微单位（默认 half-up），不是每个 SSE chunk 舍入。

示例仅为测试价格，不代表任何厂商实际收费：输入 $2/1M，缓存读 $0.5/1M，输出 $8/1M；input_total=1200 其中 cached=200，output_total=300 其中 reasoning=100。费用=(1000×2 + 200×0.5 + 300×8)/1e6 USD = 4500 微美元；reasoning 不再另加 100×8。

price_version 在入场时固定，实际服务 tier 必须可验证；无法确定实际计费维度则转待核对。公开模型别名改动不能改变旧请求价格。

## 余额与预留

posted_balance = 已提交账本余额投影；held = 活跃预留总额；available = posted_balance - held。

预留：事务锁 billing_account，检查 available >= hold，插入唯一 hold 并增加 held。预留本身不是费用；上游调用在事务外。

结算：相同锁顺序，检查 request settlement 幂等键 -> 引用 usage 与不可变 price_version -> 记平衡 journal -> posted_balance 扣费 -> held 释放 -> settlement / request 终态 -> outbox/任务，同一事务提交。

基础 settlement 每 request 唯一。纠错使用 compensating journal，不 UPDATE/DELETE 历史。每个 journal currency 内 entries 总和必须为 0（明确约定正负记法）；应用验证加数据库约束/受限存储函数或延迟触发器，普通账号不能绕过写任意不平衡分录。

## 预算上界不是随意 token 估算

prepaid 模式必须在发请求前建立可信最大收费界限。第一版只允许已知 token-based、单 choice、受限输出、无额外收费 hosted tools 的能力。必须考虑 system/tools/schema、模型上下文限制、可能的 token 计数差异和服务 tier。

有可靠 count/tokenizer 时核对其适用范围；没有可靠上界时使用保守的已配置上下文上界预留，或拒绝该组合，不能声称“字符数/4”足够。未提供输出限制的请求应用已文档化的项目默认输出上限，并把 effective 限制记入请求；用户显式超过上限返回错误而不是静默截断。

若实际收费仍超预留：记录系统异常与真实 usage，不截断 usage、不悄悄增加无授权欠款；第一版用户最多扣到已授权预算，差额记运营方损失/异常分录并关闭该能力到修复。测试验证余额不会因此穿透约束。该政策须在 API 文档披露。

## 缺失 usage 与故障

usage missing/partial -> settlement pending_reconciliation；不能直接算 0 或拿估算标 actual。用户可见待核对金额与 hold 状态。设计默认 hold 24 小时后升级人工队列，不自动当作免费成功释放；可以由有权限管理员基于证据结算或豁免并写审计分录。

请求开始前 PG 已有 request/hold/attempt。完成时 PG 不可用：有界重试写终态；不可恢复则流式输出协议错误/中止，不发最终成功；已有预留保持并由 reaper 标记未知。第一版不声称消除了“响应已发、usage 尚未持久化”的所有崩溃窗口，也不引入未经测试的本地 WAL 来作虚假保证。

reaper 只能根据已存证据推进状态，不能重发生成请求“补回结果”。不确定上游是否执行 -> dispatch_unknown。提供商无查询接口时人工处理，不能伪造精确成本。

## 幂等语义

管理充值/调整接口：相同 tenant + key + payload_digest 重放返回原安全响应；不同 payload 返回 409；完整响应如包含一次性 secret 不缓存明文，重试创建 key 返回资源元数据并提示轮换取新 key。

推理 Idempotency-Key 第一版保证网关不重复发同一逻辑请求：同 key 同摘要若已存在则返回状态/冲突指引，不重新推理；不缓存完整对话，不假装能重放原始 SSE。摘要使用租户域隔离 HMAC，避免短 prompt 可离线枚举；可选请求正文规范化必须固定版本。

exactly-once 只用于本地账本的“效果幂等”描述，不用于提供商执行。River 任务按可能重复执行设计。

## 查询与对账

汇总表是投影，可从不可变 usage/ledger 重建。每日对账检查 ledger balance、hold 合计、重复 settlement、未知请求、price_snapshot 完整性；有界时间范围按租户查。账本修复只能产生新分录，不能为了让统计一致而改原始记录。
