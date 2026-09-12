# 中断恢复

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

读取 AGENTS.md、MASTER_PROMPT.md、phases.json、progress/state.json、当前及前一阶段报告，然后检查真实源码、git diff、已有测试和 evidence。

不得因为聊天上下文丢失就重建仓库、删除数据库、重复初始化管理员或假设前面没做。若 state 与代码矛盾，以实际代码和有效证据为准，写出差异并修复状态。

先恢复最近 in_progress/blocked 阶段；implemented 阶段先 REVIEW；所有之前阶段 verified 后才推进下一个阶段。一次只做一个阶段，后续任务保留。

版本、密钥、真实上游权限等外部信息不知道就读取项目明确授权的配置或写 blocker；不得搜索无关个人凭据或临时关闭安全检查。完成可做部分并记录真实结果。
