# 阶段 09 · 授权 OAuth 框架与刷新安全

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 08 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/03-security.md`
- `docs/04-credentials-oauth.md`
- `docs/07-routing-quotas.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

实现只对明确授权提供商开放的 OAuth registry，记录 issuer/client_id/scopes/用途/来源/到期；不能复制官方客户端标识冒充接入。
实现 PKCE S256、state 摘要、主体/tenant/provider 绑定、redirect 精确匹配、一次性 claim、错误/取消/过期和秘密存储。可用 CLI exchange，不开发登录网页。
实现 singleflight + PG refresh state/nonce/version CAS，持久 request_sent 状态，重启 uncertain 处理，刷新成功原子替换凭据。
提供可运行的自有 mock OAuth server 契约测试；设备流仅有明确文档才增加。具体消费订阅 adapter 未获准则不做；框架仍需真实逻辑，不是空 interface。
写 refresh/reauth 运维流程、管理员替换与老响应竞态处理；不把重试 job 当刷新正确性。

## 交付与专属验收

交付 OAuth 完整框架、流程管理 API、refresh worker、mock issuer、双进程测试。
必须覆盖 state 重放/调包/过期、PKCE mismatch、issuer mismatch、invalid_grant、两个实例刷新、请求已消费但回包丢失、写库失败、claim 超时、旧 refresh 覆盖新 key。模拟不确定时不得第二次发旧 token。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=09 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/09.md 与 evidence/stages/09.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
