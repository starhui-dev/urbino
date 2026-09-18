# OMP 任务单（主 Agent下发；不是全阶段计划）

- task_id / phase / dependency_ids：填写真实标识。
- assigned_agent / intended_model_selector：从已验证runtime报告取，不填模型昵称代替ID。
- baseline_revision / working_directory / isolation_mode：实际值，明确共享/隔离及是否会自动应用补丁。
- goal：一个可独立验证的目标。
- non_goals：本任务不做什么。
- required_reads：AGENTS.md、本阶段prompt、必要规格与实际代码路径。
- approved_contract：输入/输出/错误类型/安全或计费不变量；禁止让worker猜测。
- allowed_write_paths：文件级或明确目录范围，避开其他任务写集。
- read_only_paths / forbidden_paths：公共契约、state、密钥和生产配置边界。
- test_ids / required_commands：对应产品矩阵ID和本机实际命令。
- evidence_directory：仅本任务可写的 evidence/tasks/NN/<task-id>/，不得写阶段总证据。
- acceptance：可观察的结果，不以“代码已写完”代替测试。
- stop_conditions：环境/授权缺失、改动越界、两轮无进展、预算/迭代达到上限。
- return：按Agent output/yield schema返回done/blocked/needs_changes、路径、证据、发现与未验证项；不写verified。

此模板字段是任务文本合同，不是 task 工具参数；只能把合同正文放入当前schema允许的 task/context 字段，不能把以上键全部作为工具JSON传入。
