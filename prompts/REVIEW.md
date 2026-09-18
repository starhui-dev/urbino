# OMP 独立复核一个阶段

主 Agent先读 AGENTS.md、MASTER_PROMPT.md、project.json、phases.json、实际 state 和当前 implemented 阶段提示词，检查任务已全部终结。指定旧阶段时明确复核对象，不改其他阶段。确认已实现文件、最终代码 revision、真实测试输出。

1. 创建一个新的 urbino-reviewer 子 Agent上下文，必须路由到已核验的 Sol；不能复用实现者上下文，不把实现者自评作为结论。提供任务规格、最终 diff、相关文件、测试源代码与证据路径。按 phases.json 的 review_agents 需要时再启动 urbino-security，两个审查均只读。
2. reviewer 检查正常/失败/并发/取消、tenant/secret/ledger/egress/retry 不变量、测试是否实际断言、证据是否旧 revision、是否有 SKIP 或 mock 冒充集成，输出具体路径、触发条件、严重性、依据与 verdict。
3. reviewer没有 shell写权限，不运行测试、不修改代码、不写 verified。主 Agent根据发现给 implementer/tester发修复任务，完成后重新跑必要检查；然后让 reviewer在最终版本再审查。任何修改使相关审查过期，不能保留旧 PASS。
4. 审查工具异常、模型不可用、资料不足要 blocked/not_run；不能悄悄改成自己写“已独立审核”。确需新主会话复核时记录真实会话来源与独立性限制。
5. 主 Agent写 progress/reports/NN-review.md 和 evidence/stages/NN.json：实际 reviewer/session/model、reviewed_revision、测试命令输出、已修复和剩余问题。只在必要项全部通过、无关键未运行项且 revision一致时标 verified。

实测命令由主 Agent或 tester执行，reviewer只能说明观察到相应证据。独立模型上下文不是第三方安全审计。复核完成即停止，不自行跳下一阶段、Git push或部署。
