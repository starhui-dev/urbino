---
name: urbino-development
description: "在 Urbino 仓库按阶段开发、恢复、测试或验收时使用；采用 OMP主 Agent规划和子 Agent实现的受控工作流。"
---
# Urbino 开发工作流

从实际仓库根目录读取 AGENTS.md 与 MASTER_PROMPT.md，再读 project.json、phases.json、progress/state.json、当前阶段提示词与required_docs。不要根据技能所在相对目录猜测工程根目录，不把本技能当实际进度。

首次接入/版本或角色变化执行 prompts/OMP_SETUP.md；开始工作使用 prompts/NEXT.md；复核使用 prompts/REVIEW.md；中断用 prompts/RESUME.md；迁移用 prompts/MIGRATE_TO_OMP.md。

Sol主会话明确边界与验收，已核验低成本子 Agent实现/测试。按 templates/OMP_TASK.md下发小任务；文件所有权、单写状态、复核独立上下文与证据真实性见 docs/15-omp-workflow.md。真实ID与配置保护见 docs/16-omp-model-routing.md。

安全/账本/取消/流式/重试不变量不因开发模型较弱而降低。高风险变更读取 docs/03-security.md、docs/04-credentials-oauth.md、docs/07-routing-quotas.md、docs/09-metering-ledger.md及阶段要求，不一次注入无关全部文档。只能对已实际验证的能力作结论。
