# 08 · 协议与适配器契约

## 第一版能力矩阵

| 下游协议与路径 | 上游 | 必须实现 | 明确不承诺 |
|---|---|---|---|
| GET /v1/models | 本地授权目录 | 当前 key 可用模型 | 任意上游全部模型列表 |
| POST /v1/chat/completions | 官方/已授权 OpenAI-compatible | JSON/SSE、文本、function tools、已验证 JSON 输出、usage | 多 choice (`n>1`)、未验证扩展、媒体/内置工具计费 |
| POST /v1/responses | 获准 Responses API | JSON/SSE、input/instructions、函数工具、output、usage、资源归属 | 私有 Codex 后端、WS、background、未实现 hosted tools、内部 compact 路径 |
| POST /v1/embeddings | 获准 OpenAI-compatible | 文本输入、有界 batch、usage | 未验证多模态输入 |
| POST /v1/messages | Anthropic API / 获准同协议 | JSON/SSE、文本块、function tools、usage/stop reason | Claude.ai 会话/OAuth 转发、伪造 thinking/签名、未验证 beta |
| POST /v1/messages/count_tokens | 同上 | 官方统计或明确 unsupported | 把字符数除常数作为官方 count |
| POST /v1beta/models/{model}:generateContent | Gemini Developer API | 文本、tools、usageMetadata、finishReason | Vertex IAM、媒体工具和未验证扩展 |
| POST /v1beta/models/{model}:streamGenerateContent | 同上 | 规范 SSE（明确 alt=sse）、分段与usage | 无证据宣称兼容所有传输格式 |
| POST /v1beta/models/{model}:countTokens | 同上 | 官方 countTokens | 估算冒充官方计量 |

任何新增特性必须同时满足：协议实现、授权、预算上界/计价、契约测试、live gate。能力描述存版本，/models 和管理员 capabilities 只展示已启用能力。

## 同协议优先

对于同协议请求，保留允许字段原始 JSON 语义与未知的无安全影响扩展；影响外部工具、URL、存储、计费或权限的未知字段默认拒绝。不能“一律透传未知字段”与严格安全策略自相矛盾；每个 provider 合约列出 allowed / forbidden / bounded-pass-through 字段。

禁止把全部 provider 强转成 Chat Completions 数据结构而丢失 Responses items、工具增量、stop_reason 或 usage。跨协议转换默认无实现，不提供假成功。

## 适配器边界（语义，最终 Go 接口由第 01 阶段定稿）

- Validate(request, capability, policy) -> validated request + budget inputs。
- BuildRequest(authorized target, credential material, validated request) -> HTTP request；仅构建，不自行发网。
- ClassifyResponse(status, bounded headers/body) -> error class / dispatch certainty / cooldown scope。
- Decode/ObserveStream -> 正确事件边界、元数据、资源 ID、usage 观察；同协议不必重编码所有事件。
- ExtractUsage -> typed usage + source/completeness，不负责收费。
- MapPublicError -> 协议兼容的安全错误结构，不泄露内部账户。

## SSE 必须通过的情况

CRLF/LF、注释、空行、多行 data、event/id/retry、UTF-8 跨 TCP chunk、单字节拆分、JSON 分段、tool args 增量、空 delta、unknown event、重复 terminal、无 terminal EOF、超大事件、上游 ping、cancel、慢下游。

不能依赖 Scanner 默认 token 大小；设置独立事件上限与内存预算。不能假设每个 TCP Read 就是一条 JSON；不能简单用字符串切 `data:`。转发保留原有事件顺序，usage 观察器不能改变有效负载或吞掉未知 provider 事件。

OpenAI Chat usage 可能出现在末尾独立块且 choices 为空；不能忽略。Responses 按规范识别 completed / incomplete / failed；不能看到任意 EOF 就加一个成功终止。Anthropic message_delta 中的某些计量是累计值，不得对每个增量盲加。Gemini 最终 usageMetadata 和候选结束原因须按合约核对。

这些字段细节必须在第 07/08 阶段再次对官方文档和真实夹具验证，不能仅凭这段概括写兼容实现。

## 流式失败与会话

客户端断开立即传播取消。处理者的 finally 只做有界结算/清理，不能转为后台继续生成。上游 EOF 且 usage 未完整 -> usage_status missing/partial；有完整 usage 但 output incomplete -> 两个状态分别记录，不混为成功。

服务端响应 ID 在发给客户端前保存 owner binding；数据库失败则不泄露无法安全续接的 ID。第一版只允许网关已见且归属校验成功的 provider resource ID；直接由外部会话带来的未知 ID 返回明确错误。

不声称通用“Codex/Claude Code 全兼容”。只记录实际测试的客户端版本、配置、协议与已通过功能；WS-only 或需要私有接口的功能明确不支持。

参考：[OpenAI SSE](https://developers.openai.com/api/docs/guides/streaming-responses)、[Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)、[Gemini token 文档](https://ai.google.dev/gemini-api/docs/tokens)。
