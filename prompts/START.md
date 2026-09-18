# OMP 首次开发入口

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json 与 phases.json。项目唯一名称 Urbino，纯后端，无前端，不依赖 CPA。

先检查实际工程；已有代码或进度时不要初始化覆盖，转 prompts/RESUME.md。如果是发行初始状态，执行 prompts/OMP_SETUP.md。通过真实 OMP 路由和烟测后，仅实现 prompts/00-foundation.md，按主 Agent计划、低成本 worker实现、主 Agent集成与证据的方式执行。完成 00 implemented 后停止，下一步使用 /urbino-review；不得自动跑完所有阶段或上线。
