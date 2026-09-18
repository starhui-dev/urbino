# OMP configuration preservation

- 配置路径命令实际输出：`/home/zhenxin/.omp/agent`，见 `config-path.txt`。
- `omp config list --json` 仅读取；未执行 set/reset、登录、升级或刷新模型。
- 主会话保持 `Hui AI (OpenAI)/gpt-5.6-sol`；项目角色只改 `.omp/agents/urbino-*.md` 的实际绑定字段。
- `omp ps` 显示当前项目无遗留 OMP 进程；所有 smoke 任务均已 terminal。
- `git-status.txt` 为当前工作区快照；未执行提交、部署或破坏性回滚。
