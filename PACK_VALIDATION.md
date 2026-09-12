# Urbino · 提示词包自身校验记录

包版本：1.1。校验日期：2026-09-12。校验对象是本提示词包，不是尚未开发的 Urbino 程序。

## 本次更新

项目正式名称固定为 **Urbino**，不设中文名、不添加品牌后缀。统一仓库/命令/镜像基础名 `urbino`、入口 `cmd/urbino/`、配置 `urbino.yaml`、环境变量前缀 `URBINO_`、真实 User-Agent、自有监控与缓存命名空间。

20 个开发阶段和 3 个续接/复核/修复提示词均加入固定命名规则。保留 14 份设计规格、164 条测试编号与全部安全/授权/计费/上线要求；扩展现有相关验收项的命名断言。进度身份字段已确定，阶段状态没有提升。

## 实际完成的检查

- `python tools/validate_pack.py`：退出码 0；阶段与依赖、文档引用、JSON/CSV、来源 URL 格式、本地链接及固定命名通过。
- `python tools/test_tools.py`：10 项辅助脚本测试通过；新增项目身份、命名漂移拒绝、配置/出口命名和全部阶段提示词命名检查。
- 00–19 共 20 个阶段分别使用 `compose_prompt.py --out` 写入临时目录并检查成功；组合文件不随包发行。
- 原有 3 份参考来源文件与上一版逐字节一致；本次没有联网重新核验其内容，不修改原先的来源核对日期。
- 所有阶段仍为 `not_started`，164 条未来网关测试要求均未执行，发布判定保持 `NOT_ASSESSED`。
- `MANIFEST.sha256` 在更新后重新生成，打包前通过 `python tools/validate_pack.py --checksums`；ZIP 另有外部 SHA-256 校验文件。

## 辅助测试真实输出

```text
test_all_phases_include_fixed_name (__main__.ToolTests.test_all_phases_include_fixed_name) ... ok
test_compose_and_refuse_overwrite (__main__.ToolTests.test_compose_and_refuse_overwrite) ... ok
test_config_and_egress_names (__main__.ToolTests.test_config_and_egress_names) ... ok
test_identity_rejects_drift (__main__.ToolTests.test_identity_rejects_drift) ... ok
test_invalid_phase (__main__.ToolTests.test_invalid_phase) ... ok
test_pack_structure (__main__.ToolTests.test_pack_structure) ... ok
test_phase_list (__main__.ToolTests.test_phase_list) ... ok
test_project_identity (__main__.ToolTests.test_project_identity) ... ok
test_review_requires_current_revision (__main__.ToolTests.test_review_requires_current_revision) ... ok
test_template_is_not_pass (__main__.ToolTests.test_template_is_not_pass) ... ok

----------------------------------------------------------------------
Ran 10 tests in 14.137s

OK
```

## 尚未进行的验证

本包不包含已实现的 Urbino 源码。未执行网关编译、Go 测试、真实上游调用、安全审计、容量压测、生产部署或备份恢复。上述辅助工具测试不代表 164 条网关验收要求已经通过。

CPA/S2A 仍只是原规格中的设计参考，不是代码依赖或安全背书；浮动引用仍须在实现阶段按真实 revision 核对。未核验域名、商标或镜像仓库可用性，也没有创建仓库或发布镜像。

## 校验发行包

解压后运行 `python tools/validate_pack.py --checksums`。MANIFEST.sha256 覆盖发行时的 61 个其他文件，不包含自身；正常开发修改内容后校验和不再匹配是预期行为，不等于网关程序测试失败。
