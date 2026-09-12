# 生产上线门禁

所有项初始均为未验证。实现者必须保存实际结果；下列 P0 不能以风险豁免绕过。

| ID | 必须满足的条件 | 证据 |
|---|---|---|
| G01 | 最终revision能重复编译，依赖/镜像锁定，无CPA/S2A依赖、无前端；项目名Urbino、命令/镜像urbino、配置与URBINO_前缀一致，无另设中文名 | build、module list、lock、naming checks |
| G02 | OpenAPI/配置/生成代码一致，无未声明兼容降级 | generate-check、contract |
| G03 | 所有资源跨租户/项目负向测试通过 | auth E2E |
| G04 | 管理/推理凭据隔离，撤销窗口、bootstrap安全 | auth/CLI |
| G05 | 凭据加密、轮换、备份解密正确，无secret泄漏 | vault/recovery/scan |
| G06 | DNS/代理/redirect/TLS/头部/可信代理安全边界成立 | transport tests |
| G07 | SSE有界、取消有效、输出后无重试、未知结果不重发 | stream E2E |
| G08 | 实际组织配额、lease、恢复epoch有效，不fail-open | two-process chaos |
| G09 | OAuth刷新不会重放已消费token；不确定状态安全停止 | refresh crash tests |
| G10 | 金额/预算/账本平衡、并发预留、幂等、unknown usage正确 | billing integration |
| G11 | 原生provider契约通过，enabled能力有逐项证据 | capability manifest |
| G12 | 真实上游授权与最小live测试、实际计量验证完成 | authorization/live |
| G13 | 安全/race/fuzz/秘密/依赖扫描无关键未处理项 | scanner logs |
| G14 | 双实例故障和容量目标有真实环境/测量 | chaos/bench report |
| G15 | 生产端口/secret/权限/反代配置经实际部署验证 | deployment smoke |
| G16 | 备份还原与升级回滚在隔离环境真实完成 | restore/rollback logs |
| G17 | 监控/告警/drain/runbook/保留策略可用 | operations E2E |
| G18 | 最终报告明确GO/NO-GO、拓扑/范围/风险，操作者验收 | RELEASE_READINESS |

对于未启用的具体 OAuth provider 可在 G09 的“具体提供商live”部分列禁用，但通用刷新引擎与mock安全测试不能省略；所有生产启用的能力都受G12约束。

任何关键检查没有环境、只有mock却要求live、没有真正还原、测试命令未跑，结果为 BLOCKED / NO-GO。上线门禁与本包 validate_pack 不是同一个东西。
