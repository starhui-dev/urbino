# OMP schema and permissions evidence

- 当前 CLI help 明确暴露 `task` 工具和 batch-capable runtime；实际任务调用使用当前 task batch schema：`context` + `tasks[]`，每项指定 agent/effort/outputSchema/schemaMode/tools。
- smoke 任务均使用唯一文件范围；scout/reviewer 仅读，implementer/tester 只写临时 scratch 文件。
- 项目 Agent frontmatter 明确 `prewalk: false`、`advisor: false`；五个项目角色均无递归委派指令。
- 任务结果均已 terminal；未遗留写任务。
