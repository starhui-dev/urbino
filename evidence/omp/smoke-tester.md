# OMP tester smoke

- Agent/session: `urbino-tester` / `SmokeTester`
- 唯一写入：`/tmp/urbino-omp-smoke/test_add.py`
- 测试：stdlib unittest，正常值、负数、str/None/float/bool TypeError，共 6 个用例
- 实际命令：`cd /tmp/urbino-omp-smoke && python3 -B test_add.py -v`
- 退出码：0；结果：6/6 OK
- 失败路径：内联注入错误断言，退出码 1；未写额外文件
- 任务终态：completed/done
