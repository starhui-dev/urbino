# OMP 官方依据与适配范围

核对日期：2026-09-18。下面文件通过官方仓库 raw 内容阅读；链接使用可读 GitHub 页面。**没有安装/运行 OMP，没有真实模型调用，没有固定 commit SHA。** 因而这是一组实现参考，不是对用户已装版本的兼容性认证；本机 setup 是强制步骤。

| ID | 官方来源 | 本包采用的机制/限制 |
|---|---|---|
| O01 | [OMP 官方仓库与 README](https://github.com/can1357/oh-my-pi/blob/main/README.md) | 工具入口、项目定位；不推断用户版本 |
| O02 | [项目提示词加载与参数展开](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/config/prompt-templates.ts) | .omp/prompts/*.md、文件名命令、description、$ARGUMENTS；同名模板优先级需实测 |
| O03 | [Agent frontmatter 解析](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/discovery/helpers.ts) | name/description/tools/model/output、prewalk/advisor；显式 tools 添加 yield |
| O04 | [原生资源发现](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/discovery/builtin.ts) | 项目/用户资源及 profile；项目 skills 目录 |
| O05 | [任务工具类型与动态 schema](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/task/types.ts) | single/batch、agent/task/context/tasks、isolated；不照搬固定 wire payload |
| O06 | [设置 schema](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/config/settings-schema.ts) | task 并发、递归、隔离与模型设置边界；不覆盖用户配置 |
| O07 | [内置 Agent 与模型角色](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/task/agents.ts) | 角色选择和模型 selector/别名；以实际版本解析为准 |
| O08 | [内置 reviewer 输出结构](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/prompts/agents/reviewer.md) | output 使用 JTD 结构；本包任务角色及文字独立编写 |

## 设计取舍

项目入口用原生 Markdown prompts，不注册会自动执行代码的扩展；Agent 是文件化角色，不用自造角色配置格式。`.omp/skills/urbino-development/SKILL.md` 按需补充工作流，不替换根指令或系统提示词。

Agent 定义保留 `name`、`description`、明确 `tools` 与小型 `output` JTD；不预填无法核实的 model ID。显式模型绑定和真实 task 元数据在 setup 验证。`prewalk`/`advisor` 未被用户版本支持时根据真实 schema 调整并记录，不猜替代字段。

原源码观察到 task schema 会随配置变化，单任务与批量任务不同。因此模板只描述任务契约；运行时读取真实工具签名，不把任务单直接当 JSON 请求发给工具。

本包不安装全局 settings，不修改认证/压缩配置，不提供 SYSTEM.md 覆盖文件。并发上限、单状态写入者与安全退出是 Urbino 的工作流策略，不冒充 OMP 默认值或 OS 级沙箱。

本次不对其他模型厂商条款、旧技术栈所有版本或参考项目安全性重新背书。详见 [技术参考范围](SOURCES.md) 和机器可读 [omp-sources.json](omp-sources.json)。
