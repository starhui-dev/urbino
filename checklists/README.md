# 清单用法

`test-matrix.csv` 为 UTF-8 BOM CSV，可用编辑器或表格程序读取。它是需求级测试清单，不是已经执行的测试结果，也不是要用表格代替自动化测试。

实际开发时填写 actual_test 与 evidence_path，保留原始 test_id 和安全目标；新增用例可追加，不能删除关键项让门禁通过。status 初始全部 not_started。

RELEASE_GATES.md 用于最终发布判定。第 19 阶段需要实现真正执行/验证这些门禁的自动化脚本。
