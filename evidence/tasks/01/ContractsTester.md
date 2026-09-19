# 阶段 01 · 契约测试任务证据（ContractsTester）

- 任务：阶段 01 测试切片，P01-T01..P01-T06（checklists/test-matrix.csv）
- 角色：urbino-tester（Stage01ContractTests）
- 日期：2026-09-18
- 写入范围：仅 `tests/contracts/**` 与本文件。未修改任何生产代码、OpenAPI、go.mod/go.sum、progress/state.json、progress/reports/01.md、evidence/stages/01.json。

## 真实命令与退出码

| 命令 | 退出码 | 结果 |
|---|---|---|
| `gofmt -l tests/contracts` → `gofmt -w tests/contracts` | 0 | json_ambiguity/openapi 两个文件曾需格式化，已格式化（仅本任务文件） |
| `go test ./tests/contracts -count=2 -v` | **1** | 46 个顶层结果（23 个测试函数 × 2 次）：42 PASS / 4 FAIL。两次运行失败集合完全一致（确定性）：`TestP01T06PublicErrorEnvelopeJSON`、`TestP01T05StrictJSONRejectsConflictingFields` 各失败 2 次 |

```
--- FAIL: TestP01T06PublicErrorEnvelopeJSON (0.00s)   （×2）
--- FAIL: TestP01T05StrictJSONRejectsConflictingFields (0.00s)   （×2）
FAIL
FAIL	example.com/urbino/tests/contracts	0.043s
```

## 测试编号映射

| 编号 | 场景 | 测试函数（tests/contracts） | 结果 |
|---|---|---|---|
| P01-T01 | 金额边界 | TestP01T01MoneyParseBoundaries；TestP01T01MoneyRejectsNegativeAmounts；TestP01T01MoneyCurrencyDeclarations；TestP01T01MoneyArithmeticOverflow | PASS ×2。覆盖：0/微单位/最大 int64 边界（9223372036854.775807）；负数、`+` 前缀、空串、非数字、双小数点、缺整数位、>6 位小数、整数位/小数位/uint64 溢出、未声明币种拒绝；币种声明集与大小写归一化；Add/Multiply 溢出与跨币种拒绝 |
| P01-T02 | 状态转移 | TestP01T02RequestTransitions；TestP01T02AttemptTransitions；TestP01T02CredentialTransitions | PASS ×2。覆盖：合法转移放行；succeeded/failed/cancelled、unknown、disabled/expired/requires_reauth 终态不可回退；pending→succeeded 等绕过拒绝；未知状态与自转移拒绝 |
| P01-T03 | OpenAPI 生成/验证 | TestP01T03OpenAPIHeaderContract；TestP01T03CommonErrorSchema；TestP01T03NoFakeSuccessOperations | PASS ×2（validated 5 operations across 3 paths）。覆盖：openapi 3.x、info.title=Urbino、info.version 非空；公共 Error schema 必填/属性恰为 code/message/request_id/retryable/details、retryable boolean、request_id uuid、details 为字符串对象、code 枚举与 internal/domain 公共码集合一致；每个操作声明 501 unsupported 且引用 Error schema、2XX 均带具体 JSON schema（无假成功占位） |
| P01-T04 | 未知配置与 production 开发开关 | TestP01T04UnknownConfigFieldsRejected；TestP01T04ProductionDevelopmentSwitchRejected；TestP01T04ShippedTemplatesMatchContract；TestP01T04ExplicitEnvironmentAllowlist | PASS ×2。覆盖：未知/拼错字段拒绝；production+development 拒绝、production+dev_mode（未声明开关）拒绝、development 允许、干净 production 放行；configs/ 两个模板可加载且 environment 正确；四个显式 URBINO_ 变量生效、未声明变量拒绝、URBINO_ENV=production 使文件内 development 失效、URBINO_HEALTH_ADDR 覆盖文件值 |
| P01-T05 | JSON 歧义 | TestP01T05StrictJSONAcceptsWellFormedConfig；TestP01T05StrictJSONRejectsUnknownAndDuplicateKeys；TestP01T05StrictJSONRejectsConflictingFields；TestP01T05StrictJSONRejectsExcessiveNesting；TestP01T05StrictJSONRejectsMultipleAndMalformedDocuments；TestP01T05StrictJSONConfigTargetIsTyped | 除 RejectsConflictingFields FAIL 外均 PASS。覆盖：合法 JSON 放行；未知字段、精确重复键、多文档、畸形 JSON 拒绝；40/100000 层嵌套拒绝且栈安全、浅嵌套放行 |
| P01-T06 | 能力契约（稳定公共错误） | TestP01T06ErrorClassesAreStable；TestP01T06ClassifiedConstructors；TestP01T06PublicErrorEnvelopeJSON | 前两个 PASS ×2；PublicErrorEnvelopeJSON FAIL ×2。覆盖：unsupported/unauthorized/unknown 三类稳定值且互异；构造器 class/code/retryable 正确、errors.As 可恢复、公共码字符串稳定；envelope 的 JSON 形状（见失败项） |

## 失败项（真实契约缺口，测试断言保留、不削弱）

1. `TestP01T06PublicErrorEnvelopeJSON` — `internal/domain/error.go` 的 `PublicError` 无 JSON 标签，`domain.UUID` 无文本序列化。实测 `json.Marshal(PublicError)` 输出：

   ```json
   {"Code":"unsupported_capability","Message":"capability not enabled","RequestID":[0,0,0,0,0,0,64,0,128,0,0,0,0,0,0,1],"Retryable":false,"Details":{"capability":"responses"}}
   ```

   契约要求公共字段为 `code/message/request_id/retryable/details`（snake_case）且 `request_id` 为规范 UUID 字符串（与 api/admin.openapi.yaml Error schema 一致，该 schema 另声明 `additionalProperties: false`）。建议修复（交回主 Agent/实现者）：为 `PublicError` 字段添加 json 标签；为 `UUID` 实现 `encoding.TextMarshaler`。`Details` 目前已是 `map[string]string`，序列化形状正确。

2. `TestP01T05StrictJSONRejectsConflictingFields` — `internal/config` `StrictJSONDecode` 的重复键检测只比较完全相同的键。实测输入 `{"health_addr":"127.0.0.1:9202","Health_Addr":"127.0.0.1:9203"}` 未报错，且经 encoding/json 大小写不敏感匹配静默命中同一目标字段、后者覆盖前者（得到 HealthAddr=127.0.0.1:9203）。违反矩阵 P01-T05"冲突字段拒绝"。建议修复：重复键检测按归一化（小写/tag 归一）比较。

## 未覆盖 / 不属本任务范围（如实记录）

- `api/admin_gen.go` 重复生成无 diff：主 Agent 的 generate-check 已覆盖，本任务不运行代码生成器。
- OpenAPI validator 工具：主 Agent 已运行并通过；本任务以真实 YAML 解析断言文档结构契约。
- CLI 级 `--config > URBINO_CONFIG > ./urbino.yaml` 优先级与缺失文件不回退：由阶段 00 `tests/foundation`（P00-T04）覆盖，未重复。
- 管理面 HTTP 行为（unsupported → 稳定 501 响应）：阶段 01 计划明确无真实管理路由，属后续阶段；本任务在 spec 级与 domain 错误级覆盖。
- 计划措辞"金额为有符号解析输入"与当前实现 `ParseMoney` 拒绝 `+`/`-` 前缀存在解释差异；本测试按可观察公共行为断言"业务金额拒绝负数"，未断言符号内部表示。

## 安全与可重复性

- 无网络、无数据库、无真实秘密；临时配置写入 `t.TempDir()`；`-count=2` 结果确定。
- 本任务终态自评 **needs_changes**（两项契约缺口待生产修复后重跑转绿）；本自评不等于阶段 verified，阶段状态由主 Agent 写入。
