# 06 · 租户鉴权、管理 API 与 CLI

## 鉴权语义

管理身份与推理用户隔离。推理 key 绑定 tenant/project/user、scope、模型策略、失效时间和 auth_version。首次请求从 PG 验证；可用短期缓存，但撤销安全窗口必须有明确上界，设计默认不超过 5 秒。高风险停用在 dispatch 前查当前状态；缓存/失效系统无法确认时拒绝，不使用无限 stale 数据。

同一 Key 可以访问其获准的多个模型；不是每模型必须发一把 key。认证中间件仅接受协议规定的入口头（Bearer、Anthropic x-api-key、Gemini x-goog-api-key）；它们都代表**网关 key**，不是用户任意传入的上游 key。禁止 URL query key。重复/冲突认证头拒绝，不选其中一个碰运气。

权限检查同时覆盖列表过滤、单项读取、更新、删除、统计、导出、任务状态和响应 ID。不存在或不归属的资源尽量统一 404，不暴露另一租户是否存在。管理员是独立 RBAC，不允许租户角色升级全局管理。

## 管理接口最小清单

统一 `/admin/v1`，独立管理 listener，OpenAPI 契约、错误类型、分页与幂等规则完整。OpenAPI `info.title` 固定为 `Urbino`，用途写入 `description`，不扩展正式项目名。

- tenants/users/projects/members：创建、读取、禁用、更新；破坏性删除改为受控注销/归档。
- api-keys：创建（秘密仅一次）、元数据读取、scope 更新、轮换、撤销；禁止取回旧秘密。
- admin-principals/tokens：最低权限授权和吊销；防止删除最后一个可用管理员；修改敏感权限需要专门 scope。
- providers/accounts/credentials：配置、禁用、受限替换；不提供秘密 export；不能让批量导入重复主体绕过配额。
- egress-profiles：配置/校验/受控连通性检查；不能成为任意 URL 测试工具。
- authorization-records/capabilities：审核记录、允许用途、有效期、功能启用状态。
- models/routes/prices：版本化发布、预览、回滚；价格发布不可变；模型改名不能重写历史。
- quotas/account-state：配额/并发设置、冷却详情、手动复核恢复；恢复须 reason，不覆盖提供商未到期的限制。
- requests/usage/ledger/holds：作用域分页查询、汇总、未结算列表、带审计人工处置。
- balance-adjustments：带原因、幂等键与外部业务参考；禁用裸余额更新。
- audit-events/jobs：受限查询；重试任务仍需幂等和授权，不能重发结果未知的模型请求。

公开只读自身接口使用 `/api/v1/me`、`/api/v1/usage`、`/api/v1/balance`；不能读取其他 key 秘密、账号和提供商内部信息。

## API 通用要求

明确的 error envelope（code/message/request_id/retryable/details-safe）；管理 mutation 使用 Idempotency-Key；有副作用更新支持 If-Match/版本 CAS，冲突 409/412 不覆盖并发修改。分页 cursor 绑定过滤条件与作用域，设 limit 上限。时间参数必须带时区，range 有上限。路径 UUID 校验、严格 content-type、JSON duplicate-key 检测；恶意模型名不能直接成为表名/列名/标签。

稳定接口错误例：unauthorized、permission_denied、resource_not_found、version_conflict、unsupported_capability、quota_exceeded、upstream_unavailable、upstream_result_unknown、billing_unavailable、credential_requires_reauth。

## CLI

唯一二进制为 `urbino`；子命令至少：serve、worker、migrate、bootstrap、version、config validate、health、admin（资源子命令）、doctor、ledger verify。帮助页和版本输出使用 `Urbino` / `urbino`，不得生成中文产品名或其他品牌名称。

除 bootstrap/migrate 外，管理 CLI 调管理 API，不直连数据库。输出支持人读表格/JSON，秘密只在明确的独占文件或显式安全输出中显示一次。PowerShell 与 POSIX 示例都要测试 quoting，不要求开发者为了 CLI 安装前端工具链。

`doctor` 只检查配置、连接、schema、密钥引用、权限和内网健康；不会扫描任意网络或请求用户未批准的外部模型。授权外部 probe 是单独操作，有预算与 rate limit。

## 无前端的边界

不创建 web/、package.json、Vue/React、静态资源、Swagger UI。OpenAPI YAML/JSON 可供下载，管理接口文档为 Markdown。OAuth 回调最小状态响应不算产品前端，也不引入登录页面。
