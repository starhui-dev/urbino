# 16 · OMP 模型路由与版本核验

目标分工来自用户偏好：Sol 负责规划、架构和最终验收；DS、GLM Flash、Luna 等低成本模型承担高 token 实现、测试或机械任务。这里的名字是偏好标签，不是可以直接写入配置的 model ID，不承诺某个服务商存在同名模型。

## 启动时必须实际核验

1. 从用户实际安装的 `omp --version`、`omp --help` 和当前会话工具列表确认 CLI 与 schema。命令失败应记录，而非自动重装或升级。
2. 只读当前 profile 与配置来源的非敏感字段。官方 README 展示的 ~/.omp/agent 路径只是常见形式，活跃 profile 可有其他路径；不得靠猜测覆盖文件。禁止原样 cat/上传 models.yml 或含认证的配置。
3. 使用当前 CLI 已支持的模型列表/会话 registry，核实 provider、id、API 类型、Base URL 指向用户既有中转、thinking 能力以及启用状态；只保存脱敏元数据。模型显示名不能证明底层实际模型身份。
4. 检查主会话保持当前 Sol。不能自动 /model、改全局默认或 fallback chain。若当前并非预期主模型，报告与最小配置 diff，暂停需要该模型的验收，不冒称已切换。
5. 检查 task、已加载自定义 agents、prompt templates 和 skills。确认全局模板没有与 /urbino-* 同名抢先匹配；发现冲突只报告并调整本项目命名或经授权合并，不删除用户全局文件。
6. 为项目 Agent 填入真实 provider/model selector，或已确认能解析的原有 task/smol/slow 别名。源码当前使用 @task/@smol/@slow；旧版本可能不同，不能把这一观察硬套到本机。遗漏 model 可能落入默认路由，不代表便宜。
7. 按 prompts/OMP_SETUP.md 完成小型读/写/测试/只读审查 smoke，记录运行元数据而非模型口头自报。不得触发网关真实上游测试。

## 建议映射，不是可直接套用配置

| 项目角色 | 偏好 | 必须核验 |
|---|---|---|
| 主会话 | 用户当前 Sol | 当前 provider/id 与 thinking，不改会话 |
| urbino-implementer | 可用的 DS Flash 类优先 | 代码工具调用、编辑、长输出是否真实可用 |
| urbino-tester | 另一已验证低成本模型；GLM Flash/DS 等 | bash/edit 与测试判断是否正常 |
| urbino-scout | GLM Flash/Luna/其他已配置低成本模型 | 限定只读，返回路径和依据 |
| urbino-reviewer | Sol | 新上下文，不能回退到实现者弱模型而隐瞒 |
| urbino-security | Sol | 新上下文，关注凭据/账本/隔离/重试 |

Luna 可作为已验证的允许回退，但不推断其提供商、价格或具体 ID。无符合角色的模型时不伪造 ID，记录路由阻塞，其他不依赖它的本地工作可继续。所有角色仅走用户已有自定义中转；不用官方登录、不引入新订阅、不选择用户排除的模型。

## 配置保护

本包不交付 .omp/config.yml、settings.json、models.yml 或 SYSTEM.md，不覆盖全局配置，不调整用户此前的 contextWindow/compaction。omp/role-policy.json 只描述本项目策略，不是 OMP 配置 schema。

确需调整时先核对本机版本/源码中的字段、备份实际文件、最小化合并、展示脱敏 diff 并做运行验证。优先绑定本项目 Agent model，不改用户全部 role。未知字段不写入、不以“文件能解析”宣称配置已生效。

来源中已观察到 task.eager、task.maxConcurrency、task.maxRecursionDepth、task.prewalk 等字段；preferred/4/1/false 是建议上限策略，不自动设置；所有与模型、并发或权限相关的实际行为以 runtime report 为准。

## 持久记录

使用 templates/omp-runtime-report.json，在 evidence/omp/runtime.json 保存实际版本、配置指纹（不含秘密）、模型路由、当前工具 schema 摘要、各 smoke 结果、证据路径和阻塞。升级 OMP、更换 model/role/协议或相关配置后重跑，旧验证不能直接沿用。

该报告是可核验索引，不是可信执行证明；check_omp_runtime.py 仅检查完整性与文件引用，不能替代读取日志和实际运行。
