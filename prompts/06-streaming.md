# 阶段 06 · 原生协议解析与流式传输基础

项目正式名称固定为 `Urbino`，不设中文名；遵守 AGENTS.md 与 docs/00-scope.md 的命名契约。

## 执行前

读取 AGENTS.md、MASTER_PROMPT.md、progress/state.json；确认前置阶段 05 已 verified。检查代码与 git diff。只执行本阶段，不越级。

## 必读文件

- `docs/05-transport-egress.md`
- `docs/08-protocols.md`
- `docs/11-testing.md`

## 目标与实际工作

实现有界 JSON 校验、SSE reader/observer/forwarder、flush 与 per-write deadline、stream idle watchdog、取消传播。
保留原事件字节顺序和多行数据；观察 usage/ID 不强制转换所有 provider payload。明确 event/JSON 深度/缓冲上限。
建立 protocol-specific 安全错误 writer，HTTP 尚未提交与已提交时行为不同。不得用统一短 WriteTimeout 杀死正常 SSE。
实装 request context 与 cleanup context 分离；无后台偷偷继续生成；timer/goroutine/连接释放路径完备。
为后续 provider 设计合成 fixtures，不下载包含真实对话或凭据的样本。

## 交付与专属验收

交付 SSE 单元/契约测试、parser fuzz 目标、慢客户端/取消测试。
覆盖单字节拆包、UTF-8、CRLF、多行 data、comments、空 delta、未知 event、工具 JSON 增量、大事件、terminal 重复/缺失、上游半关闭。go test -race 与短 fuzz 实际运行。

## 通用验收和证据

对 checklists/test-matrix.csv 中 phase=06 的每一条建立真实测试/验证记录；必要时修复前阶段回归，但不要扩大产品范围。
运行当前所有受影响的 fmt/vet/build/test/契约/集成检查，保存命令、退出码、输出与代码revision到 evidence/。未运行、缺依赖、失败和跳过分别记录。

写 progress/reports/06.md 与 evidence/stages/06.json，更新 progress/state.json 为 implemented 或 blocked；不要自行标 verified。报告中必须给出已实现能力、真实检查结果、未验证项与风险。

然后停止，由 prompts/REVIEW.md 独立复核。不要自动 Git push、调用未经批准的付费上游或部署生产。
