# 11 · 测试策略与阶段证据

## 分层

单元：领域状态、金额、权限、错误分类、SSE/JSON parser、URL policy；注入 Clock，无真实互联网。
契约：自有 httptest TLS provider 按原生协议模拟正常/异常/流式/usage/认证，夹具只含合成数据。
集成：真实 PostgreSQL/Valkey（Testcontainers 或独立明确的测试服务），跑 migrations、并发事务、Lua 和后台任务。
端到端：两个独立 urbino 进程共享 PG/Valkey，CLI 配置 -> public request -> usage -> ledger -> revoke；不是两个 goroutine 冒充两实例。
故障：kill、依赖中断/恢复、token refresh 崩溃、流式中断、worker 重放、时钟变化、Valkey epoch 丢失。
live：独立配置和授权，最少请求、明确预算；验证启用的 provider / protocol / billing dimension。

## 稳定命令

第 00 阶段建立 make fmt、vet、test、build 和 Windows 对应 Go 命令。后续补齐 lint、generate-check、test-race、test-integration、test-contract、test-e2e、test-fuzz-smoke、test-chaos、bench、security-scan、license-scan、release-gate。

未安装工具必须报失败或 NOT_RUN，不是 echo skip 后退出 0。集成测试有环境要求的 tag 可用于本地快速开发，但 release-gate 必须显式运行 tag，不能因为默认 go test 没跑而通过。

## 覆盖与断言

核心 auth/vault/billing/routing 包 statement coverage 目标 >=85%，是工程验收目标不是证明安全；还必须有清单里的负向和并发测试。fuzz 针对 parser/金额/URL，CI 做固定时长 smoke，夜间更长；所有崩溃最小样例加入回归。

`go test -race` 用真实有 CGO/工具链支持的环境；不能因为生产 binary CGO_ENABLED=0 就省略 race。goroutine、连接、timer 泄漏以重复压测/基线回归检验。

## 结果记录

每个 command 记录原始命令、开始/结束时间、环境、退出码、输出文件、测试计数（passed/failed/skipped）和对应源码 tree/commit。敏感信息先脱敏；不要日志整个 env/配置文件。

阶段状态：not_started -> in_progress -> implemented -> verified。blocked 可恢复但必须带 blocker。implemented 表示本阶段工作已做且证据收集，不意味着独立审计通过。verified 要求 REVIEW.md 对源代码/测试重新检查。

`templates/stage-evidence.json` 是结构模板，里面 exit_code:null / not_run 明确未执行。不得把模板原样复制后标 PASS。代码改动使旧证据失效，至少重跑受影响范围；发布必须对最终 revision 全量复验。

## 测试矩阵

`checklists/test-matrix.csv` 给出可跟踪编号；实现阶段新增真实测试名与路径，将 test_id -> test -> evidence 关联写入 docs/TEST_TRACEABILITY.md。不能只写一个 `TestSecurity` 空方法满足全部要求。

真凭据测试默认不进入普通 CI，不在 Pull Request fork 环境注入秘密，不将真实模型输出提交夹具。失败截图/日志不能漏 key 或真实私人代码。
