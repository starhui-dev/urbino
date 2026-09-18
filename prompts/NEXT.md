# OMP 推进一个动作

读取 AGENTS.md、MASTER_PROMPT.md、project.json、phases.json、实际 progress/state.json 与最新交接。确认 OMP 版本/配置指纹仍与 evidence/omp/runtime.json 一致，否则先 setup。

从真实状态定位第一个未 verified 阶段：implemented 则 REVIEW；blocked 则 REPAIR；not_started/in_progress 则执行该阶段提示词。明确用户指定编号也必须检查依赖。一次只实现或复核一个阶段，不能复核后自动进入下一阶段。尚有活跃子任务先收敛，不重复派写任务。

只有主 Agent更新总状态和阶段证据；任务终态与合并后测试未确认则不得推进。全部阶段 verified 时仅提示可以执行 /urbino-release，不自动部署。
