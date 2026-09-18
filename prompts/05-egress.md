# 阶段 05 · 安全 Transport、代理和 SSRF 防护

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 04 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/03-security.md`
- `docs/05-transport-egress.md`
- `docs/13-threat-scenarios.md`

## 目标与实际工作

实现唯一 TransportFactory、受控 resolver/DialContext、origin/path allowlist、TLS ServerName/CA 校验、连接池隔离与版本切换。
实现 headers allowlist/hop-by-hop 清理、禁止 redirect、凭据注入、真实 UA；客户端认证/代理/Cookie 不能透传。
HTTP CONNECT/HTTPS proxy、本地解析 SOCKS5 的行为要有模拟代理测试；无法保证代理侧目的地校验时禁用对应模式。egress 故障不回落直连。
实现入口 trusted proxy 算法和安全错误包装；限制请求体、错误正文、headers 与 connection pool。
核对 Go Transport 自身重试行为：网关 Idempotency-Key 不默认转发上游，不能把自动 GetBody/replay 留给通用客户端形成隐藏重复执行。用请求计数夹具验证推理 POST 没有未计入策略的重复发送。

## 交付与专属验收

交付 transport、resolver/proxy test server、egress config validator 与头部策略。
测试 DNS rebinding、混合 A/AAAA、metadata/IPv6 mapped、主机后缀绕过、redirect 跨 origin、错误 TLS、私网例外、代理故障无直连、跨账户 headers 不串。所有测试仅本地 mock。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=05 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/05.md 与 evidence/stages/05.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：统一出站Transport与代理配置实现 |
| 测试切片 | urbino-tester：SSRF/DNS/重定向/超时故障夹具 |
| 必须串行整合 | 主 Agent：目标验证/DNS解析与连接一致性 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
