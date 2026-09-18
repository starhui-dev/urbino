# 离线辅助工具

使用 Python 3.10+ 标准库。所有命令从仓库根目录执行；不需要 OMP 才能做离线检查，也不会访问任何模型或生产上游。

| 工具 | 用途与限制 |
|---|---|
| `validate_pack.py` | 命名、阶段、测试清单、文档链接；`--checksums` 仅用于未修改发行包 |
| `validate_omp.py` | 本包 native prompt/agent/skill 文件约定和工作流规则；不是 OMP 加载器或通用 YAML 验证器 |
| `compose_prompt.py 00 --out PATH` | 离线组装单阶段，只独占创建新文件，不覆盖已存在路径 |
| `check_evidence.py 00 --file PATH` | 原产品阶段证据格式；`--require-review --expected-revision REV` 要求当前版本复核证据 |
| `check_omp_runtime.py --file evidence/omp/runtime.json` | 实机 smoke 报告的字段/路径完备性；无法判断伪造日志，仍需审阅真实性 |
| `migration_plan.py --target PATH` | 比较发行包与已有项目，仅输出路径、摘要和合并分类，不改目标仓库 |

`python -m unittest discover -s tools -p 'test_*.py' -v` 运行辅助脚本测试；测试中的 pass 夹具是临时目录内的合成数据，不是 OMP 或网关运行证据。真实 smoke 报告模板保留 `not_run`。

## 迁移计划

新版包与已有项目必须处于互不嵌套的独立目录。命令应执行**新版包的** `tools/migration_plan.py`，`--target` 指向已有项目。stdout 输出 JSON；`--out` 只允许在两个目录之外创建一个新文件，不能覆盖。阶段状态、历史证据、测试执行状态、常见秘密文件都标 `PRESERVE_RUNTIME`，不读取其正文或摘要；其他差异标 `MERGE_REQUIRED`，而非自动覆盖。

该工具不会全盘扫描目标仓库、删掉目标独有文件、初始化进度、备份文件或替用户执行迁移。输出计划不等于写入授权。它只辅助 `prompts/MIGRATE_TO_OMP.md`；需要主 Agent 对具体变化做影响分析、备份与合并。

## 运行报告

`checked_at` 填含时区的实际 ISO 时间；`configuration_fingerprint` 是不含 API Key 的配置元数据 SHA-256，不能对秘密本身生成并公开摘要。`resolved_selector` 和 `parent_model_selector` 使用真实解析后的规范 provider/model，避免别名导致误判；路由证据以实际 OMP 元数据为准。审查角色应匹配主 Sol；低成本角色不能静默回退主模型。

每项 `evidence_path` 必须指向仓库 `evidence/` 中非空文件；可附 `evidence_sha256` 来校验内容一致性。工具不自动产生 smoke 日志，也不自动把清单改成通过。可用 `--expected-fingerprint` 对照当前已经脱敏的配置指纹；配置改变后旧 smoke 不能无条件复用。

## 完整性

`MANIFEST.sha256` 只描述发行包；开发/模型绑定之后摘要不匹配是预期情况。不要为恢复摘要而丢弃代码或修改用户配置。包脚本检查通过从来不代表账号不会封禁、代码满足产品要求或可以上线。
