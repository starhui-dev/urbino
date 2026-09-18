# OMP discovery and routing evidence

- `omp --version`：`omp/18.2.5`，见 `omp-version.txt`。
- CLI help：见 `omp-help.txt`；确认当前支持 `task`、`--mode json`、`--model`、`--no-prewalk` 等实际字段/参数。
- 项目提示词：`.omp/prompts/urbino-*.md` 共 10 个；项目 skill：`.omp/skills/urbino-development/SKILL.md`。
- 项目五个 Agent 均实际加载并完成 smoke：scout、implementer、tester、reviewer；security 文件存在并只读工具边界为 `read, grep, glob`。
- 已核验模型目录：`models.json`；低成本路由为 `Hui AI (DeepSeek)/deepseek-v4-flash` 与 `Hui AI (GLM)/glm-5.3-flash`，主 Sol 为 `Hui AI (OpenAI)/gpt-5.6-sol`。
- 显式 route smoke 的 JSON 实际返回 provider/model：
  - `explicit-deepseek-route.json`: Hui AI (DeepSeek) / deepseek-v4-flash
  - `explicit-glm-route.json`: Hui AI (GLM) / glm-5.3-flash
  - `explicit-sol-route.json`: Hui AI (OpenAI) / gpt-5.6-sol
- 自定义 Agent frontmatter 绑定与实际任务 session transcript 对应；任务返回未暴露额外 provider 元数据，故仅以显式 route JSON 和实际完成任务作为可复核依据。
