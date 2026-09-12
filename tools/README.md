# Urbino · 离线辅助工具

需要 Python 3.10 或更新版本，仅使用标准库，不访问网络，不调用 Codex，不运行模型上游。

```bash
python tools/validate_pack.py
python tools/compose_prompt.py --list
python tools/compose_prompt.py 00 --out ./urbino-phase-00.md
python tools/test_tools.py
```

默认不覆盖已有输出文件。`compose_prompt.py` 生成当前阶段、共享规则和相关规格的组合文本；也可以完全不使用脚本，直接让 Codex 阅读仓库文件。

阶段实现后再运行：

```bash
python tools/check_evidence.py 00
python tools/check_evidence.py 00 --require-review --expected-revision ACTUAL_REVISION_OR_TREE_HASH
```

上面 ACTUAL_REVISION_OR_TREE_HASH 要替换为从待发布代码实际取得的版本/树摘要，不是固定字符串。该检查只核对结构、本地文件存在性和声明一致性，不能证明日志未被伪造，也不替代重新运行测试。

新解压发行包可用 `python tools/validate_pack.py --checksums` 校验 MANIFEST.sha256。开始开发后文件会改变，这时不要期待旧发行包校验和继续匹配。

validate_pack 同时检查固定命名元数据、关键文档/阶段的命名契约和遗留占位标识。所有辅助脚本成功均只表示提示词包/证据结构与命名通过。网关上线仍须执行第 19 阶段建立的 `make release-gate`。
