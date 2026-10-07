# Agent 接入预检与最小烟测

只负责使本项目提示词、角色与当前 Agent 运行时衔接；不开始产品阶段。先读 `AGENTS.md`、`MASTER_PROMPT.md`、`docs/15-agent-workflow.md`、`docs/16-agent-model-routing.md`、`.agents/agent-policy.json`，检查 Git 状态和现有差异。

## A. 只读调查

记录运行时可提供的版本、帮助或能力清单；不存在对应接口就记录 `not_available`。只读配置来源的非敏感元数据，不输出配置正文、模型清单、密钥或环境变量值。不自动安装、升级、登录、修改全局设置或切换主会话。

确认当前会话和模型保持用户选择，项目角色只使用已核验的模型/角色选择器。检查 `.agents/prompts/`、`.agents/agents/` 和 `.agents/skills/` 是否按当前运行时能力加载；不能证明加载时记录 `blocked`，不依赖文件存在作推断。

## B. 角色绑定

按 `docs/16-agent-model-routing.md` 为五个项目角色记录实际可用的模型或角色选择器。偏好名称不是模型 ID；没有证据时保持空值。只在运行时确实支持时使用工具声明、结构化输出或隔离字段；不支持就删掉字段并保留文字约束。

## C. 有界烟测

复用用户已授权的开发路由，不访问 Urbino 真实上游或生产数据。

1. scout 只读 `project.json`，返回项目名、默认配置文件和环境变量前缀。
2. 在独立临时目录分配 implementer 实现整数加法函数，tester 编写正常、负数和类型错误测试；只使用本机已有工具。
3. 用新上下文 reviewer 只读检查文件和测试输出；reviewer 不写文件、不把模型自报当作路由证据。
4. 记录每个任务的实际会话、角色选择器、工具调用、终态和未验证项。拿不到运行元数据就标为 `unknown` 或 `blocked`。
5. 检查主会话、全局配置、秘密和未结束任务没有被改变；只清理本次创建的临时目录。

## D. 记录与完成

从 `templates/agent-runtime-report.json` 建立 `evidence/agent/runtime.json`。只填实际证据，未知项保持 `not_run`；`checklists/agent-readiness.csv` 的每一项必须有实测说明或明确未运行状态。

运行 `python tools/check_agent_runtime.py --file evidence/agent/runtime.json`。检查器通过仍需主 Agent 审阅日志真实性；失败或未知不得开始大规模开发。完成一次接入预检后停止，由 `urbino-start` 决定是否进入阶段 00。
