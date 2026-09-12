# 独立复核一个阶段

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

先阅读 AGENTS.md、MASTER_PROMPT.md、phases.json、progress/state.json 与最近 implemented 阶段的完整提示词。不要把阶段报告当作事实，读取真实实现、git diff、测试代码和运行输出。

逐项核对：本阶段是否实际落地；正常/失败/并发/取消路径；是否破坏 tenant/secret/ledger/egress/retry 约束；测试是否真的命中断言；是否跳过工具、mock 冒充集成、旧输出冒充新 revision；是否改门禁绕开失败。

重跑本阶段必需检查与影响到的回归。发现 P0/P1 或明显实现缺失就直接修复，补回归并重跑，不仅写“建议修复”。环境权限不足如实列 BLOCKED，不假装可以验证。

在 progress/reports/NN-review.md 记录具体代码路径、检查命令、证据、发现、修复、未覆盖项与结论。更新 evidence/stages/NN.json 的 review 节；只有所有必要项通过且没有关键未运行项才能 status=verified。否则 implemented/blocked，不进入下一阶段。

这里的“独立”指在新会话从事实重新核对，不代表第三方专业安全审计。最后阶段仍需上线环境与操作者验收。
