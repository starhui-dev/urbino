# Urbino · OMP 分阶段开发提示词包

版本 **2.0.0** · 2026-09-18 · 适用工具 **OMP（oh-my-pi）**

正式项目名只有 **Urbino**，不设中文名或附加产品后缀。仓库/二进制 `urbino`，配置 `urbino.yaml`，环境变量 `URBINO_`。从零开发，纯后端，不使用 CPA、CPA SDK、S2A 或其他成品中转站代码；仅借鉴公开安全工程思路。

本包是开发规格、OMP 项目提示词、Agent 定义及离线辅助工具，不是已实现的网关源码。**文件校验、OMP 工作流烟测、网关产品测试、生产上线批准是四个不同的门禁，不能互相替代。**

## 1. 新仓库：这样开始

把解压目录的内容放进新仓库根目录，**包含隐藏目录 `.omp/`**。不要只复制 `*` 而漏掉它。在该目录启动你现有的 `omp`，保持当前 Sol 主模型，然后输入：

```text
/urbino-start
```

入口先检查本机 OMP 版本、项目资源发现、实际 task schema 和已有模型配置，完成角色路由与有界烟测；通过后只开发第 00 阶段。模型、权限或工具不满足时记录阻塞，不偷偷换主模型、不让所有任务继承昂贵模型。

不知道 slash 模板是否已加载，或刚合并文件尚需重启时，直接粘贴以下普通文本，不必猜测 `/reload` 等版本相关命令：

```text
请读取当前仓库 AGENTS.md、MASTER_PROMPT.md、project.json、
omp/role-policy.json、progress/state.json 和 prompts/START.md，
按 START 的流程执行。项目只叫 Urbino；使用当前 OMP 与已有自定义中转。
先验证实际模型和子 Agent 路由，再仅推进当前允许的一个阶段。
不要覆盖全局模型/认证/压缩设置，不自动切换主会话模型、提交或部署。
```

阶段实现完成后用 `/urbino-review` 独立复核；通过后再用 `/urbino-next`。不要把它们当一个自动执行 20 阶段的脚本连发。具体失败修复和恢复规则见 [主控流程](MASTER_PROMPT.md)。

## 2. 已有代码：先迁移，不要覆盖解压

把本包解压到**工作仓库外**。在原项目的 OMP 会话中发送：

```text
请读取我刚解压的新版提示词包中的 prompts/MIGRATE_TO_OMP.md，
先确认该文件的实际路径，再对当前 Urbino 仓库执行增量迁移。
保留现有源码、progress、evidence、已执行测试记录和真实配置。
先生成差异计划及备份，再合并 OMP 工作流；不要重置任何阶段。
```

若实际路径尚未出现在会话中，附上你自己的解压路径；不要照抄不存在的示例路径。首次迁移还没有 `/urbino-migrate` 时，上面的普通文本即可。辅助工具只做只读计划：

```bash
python /实际新版包路径/tools/migration_plan.py --target /实际已有项目路径
```

已有 `AGENTS.md`、`.omp/`、模型配置不能整份覆盖。迁移后重验受影响范围，不把历史 `verified` 全部清空或无条件沿用。详见 [迁移与恢复](docs/17-omp-migration-recovery.md)。

## 3. 模型分工

| 角色 | 任务 | 模型选择原则 |
|---|---|---|
| 当前主会话 | 规划、接口/不变量、任务划分、集成和阶段裁决 | 保持用户当前 Sol |
| `urbino-scout` | 限定范围调查、定位文件和现有实现 | 已配置的低成本模型，只读 |
| `urbino-implementer` | 按冻结契约写代码、小切片实现 | 已配置的 DS / GLM Flash / Luna 等低成本模型 |
| `urbino-tester` | 边界测试、回归用例、授权的本地检查 | 已配置的低成本模型 |
| `urbino-reviewer` | 新上下文独立审查代码和证据 | 经验证的 Sol 路由，只读 |
| `urbino-security` | 认证、凭据、网络出口、账目、发布等高风险审查 | 经验证的 Sol 路由，只读 |

这些是**选择偏好，不是可直接使用的模型 ID**。五个 Agent 文件故意不预填 `model`；setup 必须从本机真实配置解析并绑定，再依据 OMP 元数据验证调用，不能靠模型自报身份。不能确认路由就停止委派。`omp/role-policy.json` 是本项目工作流规则，**不是 OMP 配置文件**，不能整份导入 OMP settings。

默认由主会话最多同时派 2 个任务，明确文件互斥且烟测成功后可增至 4 个；子 Agent 不再派生子 Agent。只由主会话更新 `progress/state.json` 和阶段结论。完整约束见 [OMP 工作流](docs/15-omp-workflow.md) 与 [模型接入](docs/16-omp-model-routing.md)。提示词约束不是 OS 沙箱；实际权限、文件隔离和 Git worktree 行为必须验证。

## 4. 项目入口

| 命令 | 功能 |
|---|---|
| `/urbino-setup` | 只做本机兼容性、模型路由和烟测，不开发产品阶段 |
| `/urbino-start` | 新项目预检后开发 00；已有进度转为恢复 |
| `/urbino-next` | 按真实进度推进一次允许动作，implemented 优先复核 |
| `/urbino-phase 07` | 指定阶段，仍校验前置条件，不绕过复核 |
| `/urbino-review` | 复核当前已实现阶段，失败时先修复再重验 |
| `/urbino-resume` | 中断/压缩后恢复，先检查尚未结束任务 |
| `/urbino-repair` | 针对当前阻塞生成有界修复任务 |
| `/urbino-status` | 只读报告阶段、证据与阻塞，不修改状态 |
| `/urbino-release` | 校验发布门禁，不等于授权部署生产 |
| `/urbino-migrate` | 合并外部新版包，不覆盖代码、状态和秘密 |

原生目录使用 `.omp/prompts/`、`.omp/agents/` 和 `.omp/skills/`。官方实现依据见 [OMP 来源与版本范围](references/OMP-SOURCES.md)。不同版本的 task schema、模型别名、profile 和资源重载方式可能不同，因此不提供猜测的全局配置或硬编码 task JSON。

## 5. 产品开发范围不缩减

保留 **20 阶段与原 164 条产品验收要求**；新增独立的 OMP 准备度清单，不冒充产品测试。

| 阶段 | 范围 |
|---|---|
| 00—04 | 工程、依赖/契约、数据库、身份权限、凭据保管 |
| 05—09 | 安全出口、SSE、OpenAI 与其他原生协议、通用 OAuth |
| 10—14 | 路由配额、计量账本、请求全链路、管理 API/CLI、后台与观测 |
| 15—17 | 安全回归、故障/多实例/性能、容器与备份回滚 |
| 18—19 | 经授权的真实联调、灰度验证、最终 GO / NO-GO |

技术底座沿用 Go + net/http/chi、PostgreSQL、pgx/sqlc、goose、Valkey、River、OpenAPI/oapi-codegen、slog/Prometheus；OpenTelemetry 按需。**本次不升级语言、依赖或镜像版本**：旧包中的具体补丁版本只是历史基线，第 00/17 阶段必须按当时官方资料核对、固定版本与摘要，不能用 `latest` 或虚构已验证版本。

不增加前端、公开注册、在线支付、动态插件或微服务。协议支持边界和不支持能力仍以 [范围](docs/00-scope.md) 为准。

## 6. 安全与验收

保持凭据隔离、刷新协调、组织配额、有限重试、会话归属、SSRF 防护、敏感信息脱敏、未知用量、幂等结算和账本规则。不得用客户端身份伪装、挑战绕过、撤销令牌重用或轮换账号逃避停用/限额。**这些工程措施不构成不封号保证**；具体上游授权和条款在启用前重新核对，历史参考不自动变成授权。

开发阶段默认只使用 mock 与本地测试。开发模型经现有中转调用和 Urbino 真实上游联调是不同权限：后者须单独确认凭据、环境与预算。没有实测证据不能标 `verified`，没有上线门禁证据必须 `NO-GO`。新上下文模型审查也不等于第三方安全审计。

## 7. 工具与文件

Python 辅助工具只依赖标准库，要求 Python 3.10+；它们不会调用模型、安装依赖、修改目标仓库或部署。

```bash
python tools/validate_pack.py
python tools/validate_omp.py
python -m unittest discover -s tools -p 'test_*.py' -v
python tools/compose_prompt.py 00 --out ./urbino-phase-00.md
```

首次开发 smoke 之后才运行：

```bash
python tools/check_omp_runtime.py --file evidence/omp/runtime.json
```

空白 `templates/omp-runtime-report.json` 应当被检查器拒绝；不能为了运行通过而直接把模板字段全改成 pass。原始发行包可用 `python tools/validate_pack.py --checksums` 校验，开始开发或配置 Agent 后摘要变化是正常的，不要据此回滚用户改动。

目录重点：`AGENTS.md` / `MASTER_PROMPT.md` 管全局规则；`phases.json` / `prompts/00-19` 管阶段；`.omp/` 管原生入口；`omp/` 管项目策略；`templates/OMP_*` 管任务交接和报告；`progress/` 是新项目初始状态。

[实际包校验记录](PACK_VALIDATION.md) · [变更记录](CHANGELOG.md) · [工具说明](tools/README.md) · [原技术参考](references/SOURCES.md)

本次制作环境没有安装 OMP，没有执行真实模型路由/子 Agent 烟测，也没有运行尚未实现的网关。所有实机检查留到你已有 OMP 环境的 setup，不把离线校验冒充实机验证。
