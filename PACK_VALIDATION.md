# Urbino · OMP 提示词包验证记录

提示词包版本：2.0.0 · 检查时间：2026-09-18T04:41:02+00:00

## 已实际执行

| 检查 | 结果 |
|---|---|
| `python tools/validate_pack.py` | 通过：命名、文件、Markdown 链接、20 阶段依赖、164 条产品测试要求 |
| `python tools/validate_omp.py` | 通过：10 个原生提示词、5 个 Agent、1 个 skill、24 条 OMP 准备度要求 |
| `python -m unittest discover -s tools -p 'test_*.py' -v` | **46 项通过**；临时夹具测试，不是实机证据 |
| 00—19 阶段提示词逐个组装 | 20/20 成功；输出已存在时拒绝覆盖 |
| 原产品矩阵比对 | `checklists/test-matrix.csv` 与原包逐字节一致，164 条完整保留 |
| 原阶段依赖/名称 | 20 阶段 ID、标题、依赖与原包一致 |
| 命名与状态 | 项目仅 Urbino、无中文名；发行状态仍为全部 not_started |
| YAML frontmatter 离线解析 | PyYAML 解析 16 个资源；检查 Agent 输出字段结构；不是 OMP 运行解析器 |
| Python 语法 | 全部辅助脚本成功解析 |
| 空运行报告/阶段证据 | 检查器均按预期拒绝模板，返回非零 |
| 迁移保护回归 | 验证目标内容不变、状态保留、秘密不读、越界/符号链接受控、拒绝覆盖报告 |
| ZIP 完整性与重新解压 | `testzip()` 无损坏；隐藏 `.omp/` 存在；重新解压后结构、摘要与46项测试通过 |

产品测试矩阵 SHA-256：

```text
410da800d9c513e1c07474e27a2b19c8dfcd80e51dad3ab542c88a0597658b75
```

辅助单测原始输出见 [pack-tests.log](validation/pack-tests.log)，机器可读离线结果见 [offline-summary.json](validation/offline-summary.json)。这些位于 `validation/`，不冒充 `evidence/stages/` 中未来产品证据。

## 未执行，不能据此宣告通过

本次制作环境中 `omp` 和 `bun` 不在 PATH，没有安装 OMP、启动 Agent 或调用任何开发模型。**真实资源加载、角色模型绑定、provider/model 元数据、任务闭环和权限边界均未实机验证**，须在用户现有 OMP 中执行 `/urbino-setup`（`/urbino-start` 会先执行此流程）。官方源码阅读也不能替代用户已安装版本的兼容性测试。

五个 Agent 的发行模板不预填模型 ID；这是为了不伪造用户配置。setup 必须绑定并验证实际路由后再开始开发。没有运行网关构建/集成测试、真实上游联调、负载测试、恢复演练或生产部署。164 条产品测试与24条 OMP 准备度要求均未在真实开发环境执行。

`check_omp_runtime.py` 只能检查记录结构、路径与可选摘要，不能鉴别人工编造的日志。模型调用身份需 OMP 元数据、阶段实现需真实代码与检查输出，最终上线仍须原发布门禁与操作者批准。

## 发行摘要

`MANIFEST.sha256` 列出本包所有其他文件，包含 `.omp/`；ZIP 外另有整体 `.sha256` 文件。配置或开发以后文件变化会使发行摘要不匹配，这是正常的，不能因此回滚用户代码或模型设置。
