# OMP 接入预检与最小烟测

只负责使本项目提示词、角色与已有 OMP 配置正确衔接；不开始其他产品阶段。读 AGENTS.md、MASTER_PROMPT.md、docs/15-omp-workflow.md、docs/16-omp-model-routing.md、omp/role-policy.json，检查仓库 Git 差异。

## A. 只读调查与备份

运行实际可用的 omp --version/--help；按本机 help 查询模型和会话工具/agents。确认真实配置来源/profile，只检查 provider/id/role/base URL host/协议类型等脱敏元数据；不要把 key 或完整配置打印给模型。不自动升级、登录、新增 provider 或改用户的 compaction。

确认主会话仍是用户原 Sol，所有开发模型走用户已有中转。检查项目 /urbino-* 模板是否加载及同名冲突；检查五个 Agent 与 skill。不同版本的 agents/model alias/工具 schema 有差异，以当前真实 schema 为准，读取源码不能替代运行验证。

任何修改先备份文件及当前指纹，备份路径放受限本地目录，不打印秘密；只合并本项目文件。现有全局设置未被用户授权修改则只输出最小建议，不动它。

## B. 绑定项目角色

根据已配置可用模型，给 .omp/agents/urbino-implementer.md / urbino-tester.md / urbino-scout.md / urbino-reviewer.md / urbino-security.md 的 model 字段填写真实 selector，或已经验证的别名；不得把 DS、GLM Flash、Luna 等偏好文字直接当 ID。reviewer/security 使用 Sol，其他按低成本分工。当前默认模型不改。

校验本版本支持 tools/output/prewalk/advisor 的用法。模板的 prewalk/advisor=false 是减少隐含调用的策略；字段不支持时删去该字段并在 prompt 中保留禁用约束，不自造替代字段。验证无隐含 advisor 调用、无再次派生 task；output/yield 按实际 schema。不将任何 API Key 放进 Agent 文件。

刷新方式先读本机帮助；需要时重新进入仓库启动 OMP，不能假称热加载成功。新版文件尚未加载就停止并给出唯一的恢复命令，不能继续使用旧 Agent。

## C. 有界烟测

这是开发模型调用，复用用户已经授权的开发路由；不访问 Urbino 的真实上游、不执行生产测试。

1. 委派 scout 只读 project.json，返回 project_name/default_config_file/environment_prefix 及来源路径。真实结果必须 Urbino/urbino.yaml/URBINO_。
2. 在独立 scratch 目录分配 implementer 实现一个整数加法小函数、tester编写正常/负数/类型错误测试；尽量用本机已有 Python 或已验证工具，不为烟测下载依赖。各自只写分配文件。执行测试，主 Agent读实际文件和输出。
3. 用新上下文 reviewer 阅读这两个文件和运行输出，报告 verdict/事实；reviewer不得写入或自报执行了测试。高风险 reviewer role 可复用同一模型解析证明，但仍验证 security Agent 被加载且权限符合要求。
4. 按 OMP 实际返回元数据核对每个 task 的 provider/model、独立 session、终态。不得相信模型回答“我是某型号”。无法取到实际路由证据就标 unknown/blocked，不宣布闭环完成。
5. 检查模型设置和主会话未被改变，无未结束任务、无秘密输出；scratch 仅能清理此次创建的目录，保留必要脱敏日志和差异。

## D. 记录与完成

从 templates/omp-runtime-report.json 建立 evidence/omp/runtime.json；填实际证据、角色解析结果、配置指纹、日期和阻塞。checklists/omp-readiness.csv 每项有实测说明或明确 not_run；不套用包内静态测试结果。

运行 python tools/check_omp_runtime.py --file evidence/omp/runtime.json。检查器通过还需要主 Agent读日志确认真实性；失败或未知不得开始批量开发。报告预检结果、最小修改、真实模型分工与阻塞。

独立 /urbino-setup 完成后停止；由 /urbino-start 调用时，仅在全通过后继续 00 阶段，已有工程则按当前 state 选择继续动作，不能重置。
