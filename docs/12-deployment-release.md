# 12 · 部署、安全发布与真实上线标准

## 两种交付方式

A. 单机 Compose：urbino + urbino-worker（可合并）+ PostgreSQL + Valkey，反向代理可由 1Panel/既有 Nginx 提供。不要求安装 Kubernetes。
B. 外部依赖：urbino/urbino-worker 连接已提供的 PG/Valkey，文档明确 TLS/CA、账号、迁移权限、备份责任与网络策略。

单机 Compose 不是 HA。若要称多实例生产支持，必须完成两进程共享状态故障测试；若要称存储高可用，必须另外验证实际 PG/Valkey HA 拓扑，不能由程序多副本推导。

## 容器

镜像基础名称为 `urbino`；默认 Compose 项目名 `urbino`，应用服务 `urbino` 与 `urbino-worker`，两者使用同一镜像分别执行 `urbino serve` / `urbino worker`。镜像 registry/owner 未指定，不虚构可拉取地址；实际版本与 digest 在本阶段锁定。容器内配置路径约定 `/etc/urbino/urbino.yaml`，显式只读挂载；不为重命名变更 PostgreSQL 表名、上游协议字段或第三方配置。

多阶段构建、非 root、只读根文件系统、drop all capabilities、no-new-privileges、有界 tmpfs、无 Docker socket、resource limit、明确 SIGTERM。secret 通过受限挂载/secret file；镜像与构建依赖锁定 digest/version；amd64/arm64 分别编译及最小运行测试，未经运行不宣称全平台通过。

PG/Valkey 不映射公网端口。urbino host 端口默认绑定 127.0.0.1，经 TLS 代理访问；远程管理不通过同一个 public upstream location。容器网络与 DNS policy 要使出站 SSRF 防护成立。

## 迁移与备份

独立 migrator 角色执行显式 migration，应用无 DDL 权限。发布前备份、验证 restore、锁定迁移版本；应用启动拒绝不兼容 schema。rollback 优先回滚应用到兼容版本，不自动执行有损 down。

备份内容包括 PG、必要配置、**独立保护的密钥材料**；只有数据库密文但丢了密钥不能恢复。密钥与密文备份分开控制、加密传输和保存。备份恢复在隔离环境验证租户查询、secret 解密、账本核对、管理员撤销状态；不能只检查 backup 文件存在。

工程目标（不是实测/SLA）：RPO <=24h、RTO <=60min 的单机基线；需要更低 RPO 时增加 WAL/PITR 设计与实际测试。任何实际测量超标均须记录，不能写目标当结果。

## 性能验收基线

建议自有 mock 测试环境：urbino 4 vCPU/8 GiB、独立 PG/Valkey，记录具体硬件与网络；先测试 200 个并发 SSE（每 100ms 一个小事件、每流 30s）、同时 30 RPS 短请求、持续 10 分钟，再按资源扩压。mock 凭据不影响真实账户。

目标：无 tenant 越权/重复收费；网关自身增加的短请求 p95 延迟 <=50ms（需直连 mock 基线相减）；无持续 goroutine/内存增长；受控拒绝而非 OOM。以上都是待验证目标，不是本包已测性能或通用容量承诺。

压测报告含 p50/p95/p99、TTFT、成功/主动拒绝/错误区分、RSS/CPU/GC、DB pool、Valkey 延迟和连接数。不能把总耗时中的模型生成时间算成网关延迟。

## 灰度与停止条件

先 staging -> 单测试租户 -> 少量实际流量 -> 明确观察窗口 -> 扩大。每步由运营方确认，不自动修改生产权重。出现 secrets 泄漏、跨租户、账本不平、重复扣款、账户停用激增或恢复闸门失效立即停止扩量并按 runbook 处理。

## 发布文件

Dockerfile、Compose（内置/外部两版）、Nginx SSE 示例、配置 schema/示例、迁移说明、ADMIN_API.md、CLIENT_COMPATIBILITY.md、RUNBOOK.md、BACKUP_RESTORE.md、UPGRADE_ROLLBACK.md、SECURITY.md、SBOM/扫描结果、RELEASE_READINESS.md。

启用能力清单逐项列：provider、认证模式、协议、model family、billing dimensions、client version、授权引用、live result、部署拓扑。不存在“全兼容”复选框。

## GO / NO-GO

GO 需要：所有 00-17 阶段 verified；18 阶段对至少一个真正启用的上游完成授权+真实请求+账务比对；没有生产启用的能力缺 live 证据；安全/故障/还原/回滚门禁通过；19 阶段独立复核通过。

没有真实凭据时可以做到“实现与 mock 验证完成”，但最终结论必须 NO-GO（缺 live evidence），不能伪造上线报告。关键项目 BLOCKED 不接受豁免；非关键项只有明确禁用且不属于发布范围时才可 N/A，并说明原因。
