# OMP implementer smoke

- Agent/session: `urbino-implementer` / `SmokeImplementer`
- 唯一写入：`/tmp/urbino-omp-smoke/add.py`
- 契约：两个非 bool Python int 返回和；非 int 或 bool 抛 `TypeError`
- 实际命令：Python smoke 覆盖正常、负数、str/None/float/bool 类型错误
- 退出码：0；输出：所有检查通过
- 仓库文件、进度、门禁、OMP 配置：未修改
- 任务终态：completed/done
