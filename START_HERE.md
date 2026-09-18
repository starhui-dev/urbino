# Urbino · 先看这里

新项目：将整个包（**包括 `.omp/`**）放入仓库根目录，启动现有 OMP，保持当前 Sol 主会话，发送 `/urbino-start`。

它先检查真实 OMP 版本和已配置模型，绑定项目角色并实测路由，再仅开发一个阶段。不会替你填写虚构的 DS/GLM/Luna 模型 ID，也不覆盖全局模型/认证配置。实现后用 `/urbino-review`，通过后再 `/urbino-next`。

模板尚未加载时直接发送：

```text
请阅读 AGENTS.md、MASTER_PROMPT.md、project.json、omp/role-policy.json、
progress/state.json 和 prompts/START.md，并执行 START。
保持当前 Sol 主会话，其他开发模型复用我已有中转。
先做实际 OMP/角色预检，成功后只推进当前允许的一个阶段；
不硬编码模型 ID，不覆盖用户配置、进度或证据，不自动提交或部署。
```

已有代码：**不要把包覆盖解压到工作仓库**。在仓库外解压，提供实际解压路径，让当前 OMP 阅读并执行新版 `prompts/MIGRATE_TO_OMP.md`，先做备份和差异计划。

[完整说明](README.md) · [迁移提示词](prompts/MIGRATE_TO_OMP.md) · [实际验证范围](PACK_VALIDATION.md)
