# 公共协议字段契约

本文件与 `internal/protocol` 的 `PublicEndpointPolicies` 同步。所有列出的能力初始为 **disabled**；即使请求已通过网关身份认证，未启用能力仍返回稳定的 `unsupported_capability`。未通过身份认证返回 `unauthorized`，两者不能混淆。当前阶段不创建公共 listener。

每个 JSON body 使用严格解析：最大 8 MiB（count 端点 2 MiB）、最大嵌套深度 64、UTF-8 必须有效、对象键在 JSON 解码后不得重复（包括转义后相同的键），且顶层只能有一个值。未知字段和 forbidden 字段拒绝；bounded pass-through 仅限下表字段、保留 `json.RawMessage` 原始语义，并有 16 KiB 上限。嵌套媒体、托管工具、远程文件或执行字段一律拒绝，工具只允许后续阶段经授权的客户端函数工具契约。

| 方法与路径 | allowed | forbidden | bounded pass-through |
|---|---|---|---|
| GET `/v1/models` | 无 body | 查询参数与 body | 无 |
| POST `/v1/chat/completions` | model, messages, stream, temperature, top_p, max_tokens, max_completion_tokens, stop, seed, response_format, tools, tool_choice, user | n, modalities, audio, images, image_url, video, web_search_options, service_tier | metadata |
| POST `/v1/responses` | model, input, instructions, stream, temperature, top_p, max_output_tokens, tools, tool_choice, previous_response_id, store, include | background, conversation, computer, file_search, web_search, code_interpreter, hosted_tools, compact | metadata |
| POST `/v1/embeddings` | model, input, encoding_format, dimensions, user | image, images, input_audio | 无 |
| POST `/v1/messages` | model, messages, max_tokens, system, stream, temperature, top_p, top_k, stop_sequences, tools, tool_choice | thinking, container, service_tier, betas, cache_control | metadata |
| POST `/v1/messages/count_tokens` | model, messages, system, tools | thinking, container, betas | 无 |
| POST `/v1beta/models/{model}:generateContent` | contents, systemInstruction, generationConfig, safetySettings, tools, toolConfig | cachedContent, fileData, inlineData, codeExecution, googleSearch, urlContext | metadata |
| POST `/v1beta/models/{model}:streamGenerateContent` | 与 generateContent 相同 | 与 generateContent 相同 | metadata |
| POST `/v1beta/models/{model}:countTokens` | contents, systemInstruction, tools | cachedContent, fileData, inlineData, codeExecution, googleSearch, urlContext | 无 |

`FindEndpointPolicy` 只匹配上述固定方法与路径（Gemini 的 `{model}` 仅允许单个非空路径段）。不在此表中的 API 保持稳定 `unsupported`，不能根据客户端字段猜测兼容协议或转换成其他协议。provider 适配端口只负责校验、构造已授权请求、分类响应和观察 usage，不自行发起网络请求。
