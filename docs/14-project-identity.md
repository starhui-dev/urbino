# 14 · Urbino 项目命名与标识契约

项目唯一正式名称为 Urbino；机器字段 display_name 同样为 Urbino，chinese_name 必须为 null。名称不翻译、不音译。本文件约束所有阶段，不重新选择产品名，不改变原有功能范围。机器可读来源为 [project.json](../project.json)；实现、文档、示例和发行产物必须一致。

## 统一标识

| 用途 | 约定 |
|---|---|
| 唯一正式名称 | **Urbino**；不设中文名，不附加产品后缀 |
| 仓库、镜像基础名称 | `urbino` |
| Linux/macOS 二进制 / Windows 二进制 | `urbino` / `urbino.exe` |
| Go 命令入口 | `cmd/urbino` |
| Go module 默认占位 | `example.com/urbino` |
| 用户配置文件 | `urbino.yaml` |
| 开发 / 生产配置模板 | `configs/urbino.example.yaml` / `configs/urbino.production.example.yaml` |
| 应用环境变量 / 配置路径变量 | `URBINO_` / `URBINO_CONFIG` |
| 默认 Compose 项目 / API 服务 / Worker 服务 | `urbino` / `urbino` / `urbino-worker` |
| 容器内配置路径 | `/etc/urbino/urbino.yaml` |
| 应用 User-Agent | `urbino/<version>`，version 来自实际构建信息 |
| 应用自有 Prometheus 指标前缀 | `urbino_` |
| OpenTelemetry service.name | `urbino`，角色使用受控属性区分 |
| Valkey 键命名空间 | `urbino:<deployment-id>:`，再追加原规范所需的租户/配额等隔离信息 |

`example.com/urbino` 仅是本地初始化占位；未声明真实域名、仓库地址或镜像 registry/owner。若当前仓库有用户已确认的正式 module 路径，则使用正式路径并同步 project.json、导入、构建和文档。确认前不猜测 GitHub 组织、不创建远程资源、不声称示例镜像可以拉取。

名称只用于本项目，不宣称是游戏官方产品或已获得关联授权；不自动添加游戏图标、角色素材或品牌 Logo。此包不包含商标或域名可用性调查结论。

## 命令与配置

命令统一从 `urbino` 入口进入，先定义契约，再按对应阶段实现；未实现命令必须明确失败，不能提供假成功占位。

```text
urbino version
urbino serve
urbino worker
urbino migrate
urbino bootstrap --output <new-file>
urbino admin ...
urbino doctor
urbino config validate
urbino admin ledger verify
```

管理操作通过管理 API；migrate 与首次 bootstrap 保持既有特殊权限边界。`doctor` 不得绕过管理鉴权读取受限数据；离线 `config validate` 不发起真实上游请求。

配置路径选择顺序：显式全局参数 `--config` > `URBINO_CONFIG` > 当前目录 `./urbino.yaml`，只选择一个文件，不隐式合并不同目录下的配置。指定文件不存在时明确报错。应用参数如需支持环境覆盖，须在第 01 阶段 schema 中逐一列出允许的 `URBINO_` 变量及优先级；秘密继续使用受限文件/KMS，不放到命令行参数中。

Linux 和 PowerShell 示例同时验证；Windows 允许 `urbino.exe` / `./urbino.exe`，不强迫使用 Unix 绝对路径。文档中的 `<path>`、`<version>`、`<deployment-id>` 均为必须用真实值替换的占位，不是可直接上线的配置。

## 重命名边界

只修改本项目名称、命令、目录、应用配置、镜像及应用自有命名空间。不要改写：

- OpenAI/Anthropic/Gemini 等外部协议字段、API 路径、官方版本头、模型 ID 或第三方 URL。
- OAuth、Prometheus `go_*` / `process_*`、标准 `OTEL_*` 配置等外部约定；这些不属于应用自有 `URBINO_` 环境变量或 `urbino_` 指标。
- CPA/S2A 的参考文档、来源名称或引用 URL。保留参考不等于引入实现。
- PostgreSQL 表和列、既有 Key、凭据密文、已有账务数据，不能为了改名执行批量数据变更。

应用身份头只表达真实 Urbino 版本；不伪装官方客户端、不借改名修改安全策略。Valkey namespace 中 deployment-id 在同一部署的所有实例间一致，不同部署间隔离；不能把部署名替代租户与组织配额隔离。

## 阶段责任与验收

00：初始化 module、cmd/urbino、版本与配置入口，建立命名检查。
01：固化配置 schema、模板路径、环境映射和管理 API 的 `info.title=Urbino`。
05：验证真实 User-Agent；不修改上游协议身份与授权约束。
13：CLI/帮助/操作文档统一，Linux 与 Windows 示例通过。
14：应用指标、OTel 服务名称及告警表达式一致。
17：镜像、Compose、配置挂载、发布产物与运行说明一致。
19：检查以上名称的端到端一致性并保留证据，不因名称检查通过宣称生产就绪。

本次不增加阶段或替代原 164 条测试要求；把命名用例附加到对应阶段的真实测试与证据中。包内辅助检查仅验证规范一致性，无法证明尚未开发的 CLI 或配置解析器实际存在。

## 已有工程

已开始开发时按 [命名更新提示词](../prompts/APPLY_NAMING_UPDATE.md) 增量合并，保留进度和用户代码。存在活跃服务、已有缓存 namespace 或旧配置时先做影响分析与迁移计划，不自动改生产或默默接受所有旧变量作为别名。
