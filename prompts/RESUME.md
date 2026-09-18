# OMP 中断恢复

先读 AGENTS.md、MASTER_PROMPT.md、project.json、phases.json、实际 state、最新 progress/handoffs/ 与阶段/任务报告，检查 Git diff 与当前 OMP真实任务状态。不要依靠会话摘要覆盖磁盘事实。

仍有子任务活跃时先等待/收集或安全终止并记录差异，不重复派发同一写任务。确认文件所有权、已合并/未合并补丁、测试实际 revision 与未跟踪文件。不要删除临时工作树或重置用户修改。

OMP版本/model/profile或相关配置变动则重跑 setup，保留所有旧阶段事实。implemented 先 REVIEW；blocked 先 REPAIR；其余只恢复当前阶段剩余工作。不重建完整项目、不从00重新开始。更新交接并在一次阶段动作完成后停止。
