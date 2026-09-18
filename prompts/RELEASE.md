# OMP 上线候选检查

读取 AGENTS.md、MASTER_PROMPT.md、phases.json、progress/state.json、checklists/RELEASE_GATES.md、docs/12-deployment-release.md 与 prompts/19-release-audit.md。首先只读核对是否满足进入第19阶段的依赖；不满足则输出 NO-GO和缺项，不越级。

按19阶段进行真实证据复核；使用全新 urbino-reviewer 与 urbino-security 上下文。核对启用功能的授权/live tests、计量账本、秘密隔离、备份还原、升级回滚、目标拓扑与监控；OMP烟测不能替代这些。

当前授权范围内可以执行本地候选构建/测试；任何真实付费调用、生产迁移和部署仍须另外明确批准。最终产物是 docs/RELEASE_READINESS.md 和证据，GO也不触发自动部署。
