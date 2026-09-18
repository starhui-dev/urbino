# 阶段 04 · 上游凭据加密与轮换

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、project.json、progress/state.json；确认前置阶段 03 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/15-omp-workflow.md`

- `project.json`
- `docs/14-project-identity.md`
- `docs/02-data-model.md`
- `docs/03-security.md`
- `docs/04-credentials-oauth.md`

## 目标与实际工作

实现 AES-256-GCM keyring、凭据加解密、AAD 规范、nonce 生成、key_id/version、只读秘密访问端口；把代理密码同样交给 vault。
实现管理员安全导入 API key、轮换/禁用，普通 GET/list 不解密输出；不接入消费订阅 Cookie 导入。
实现 keyring 轮换任务的 checkpoint/CAS、读取旧新 key 兼容、失败恢复；编写密钥备份与丢失的处理说明。
禁止把 secret 放日志、trace、String()/fmt 输出、测试 golden；错误只暴露可处理的安全分类。限制关键 struct 的序列化。
权限验证与生产启动检查完整，不宣称 Go heap 内存能完全擦除。

## 交付与专属验收

交付 vault 实现、受限 key file loader、rotation 命令/API、加密数据库集成测试。
测试 tamper、wrong key/AAD、跨 tenant/account ciphertext 交换、随机 nonce、并发 version update、轮换中断、旧备份可恢复；使用合成密钥搜索日志与数据库明文字段。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=04 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

由主 Agent 写 progress/reports/04.md 与 evidence/stages/04.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。

## OMP 本阶段编排

主 Agent先固定契约和任务范围；仅在当前 OMP路由预检通过后委派。

| 工作 | 分配 |
|---|---|
| 实现切片 | urbino-implementer：凭据保险库/AEAD/版本轮换实现 |
| 测试切片 | urbino-tester：错AAD/错Key/轮换竞态/日志泄密测试 |
| 必须串行整合 | 主 Agent：加密格式/秘密源/轮换事务 |

先确认可独立文件边界，默认2个任务且总上限4；存在共同写文件/公共契约则串行。任务单使用 templates/OMP_TASK.md，结果按 templates/OMP_WORKER_REPORT.md 收集，主 Agent在最终合并后的工作区重跑检查。
本阶段独立审查角色：urbino-reviewer、urbino-security。必须是新上下文，不让实现者自行验收；高风险发现修复后重跑并重审。只有主 Agent写阶段总状态。
