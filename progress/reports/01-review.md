# 阶段 01 · 独立复核

复核对象为阶段 01 的实际源码、测试、生成文件和工作树；原阶段证据记录的 `f39e042` 已过时，当前基线为 HEAD `20cb1aec4e8f132fbf148bb30fae55d4782a36be` 加本次工作树修复。

复核发现并修复了以下问题：

- `internal/config.validate` 原先允许 production 缺失 `database.url_file`/`valkey.url_file`，以及空 provider allowlist 项和无币种的非 prepaid 账本；现已拒绝，并在 `internal/config/config_test.go` 增加回归测试。
- `isPublicListen` 原先只拦截少数通配地址，公网 IP 和主机名可能通过 production 管理监听校验；现改为仅允许 loopback IP，未知主机名和非 loopback 地址均拒绝，并增加测试。
- 阶段提示要求的 `TransportFactory`、`Vault`、`UsageObserver`、`Reservation`、`Scheduler` 端口原先缺失；已在 `internal/provider/contracts.go` 以最小接口补齐。新增未知 capability/scope 枚举拒绝测试。
- `Usage` 原先未校验 `usage_source` 枚举及 `is_estimate` 一致性；已补充 `UsageSource.Valid`、一致性校验和回归测试。
- 配置 schema 将账本模式/币种限制为 `disabled`/`prepaid` 与大写三字母代码，加载器现对开发和生产环境执行同一枚举/格式校验。
- 新增未知公共 endpoint 不被 capability 目录广告的回归测试。

真实检查结果：

- `go test -count=1 ./...`：exit 0，所有测试通过，provider 包无测试文件且无 skip（`evidence/raw/review-01-go-test.txt`）。
- `go vet ./...`：exit 0（`evidence/raw/review-01-go-vet.txt`）。
- `go build -o urbino-review.exe ./cmd/urbino`：exit 0（`evidence/raw/review-01-go-build.txt`）。
- `go generate ./api`：exit 0；随后 `git diff --check` exit 0，生成文件无差异（`evidence/raw/review-01-generate.txt`、`review-01-diff-check.txt`）。
- 开发和生产样例分别执行 `urbino config validate`，均 exit 0（`evidence/raw/review-01-config-dev.txt`、`review-01-config-prod.txt`）。
- `ConvertFrom-Json` 解析 `configs/urbino.schema.json`：exit 0（`evidence/raw/review-01-schema.txt`）。
- 安装 MSYS2 MinGW-w64 GCC 后，在当前进程设置 `CGO_ENABLED=1`、`CC=gcc` 并将 `C:\msys64\ucrt64\bin` 加入 PATH，`go test -race ./...` exit 0（`evidence/raw/review-01-go-race.txt`）。
- `uv run --python 3.14 python tools/check_evidence.py 01 --require-review --expected-revision ...`：exit 0，证据结构和本地引用通过（`evidence/raw/review-01-check-evidence.txt`）。

P01-T01 至 P01-T06 均有真实测试或生成/配置检查；未接入 PostgreSQL、Valkey、provider、真实上游和流式端到端路径属于后续阶段，不能据此宣称集成或 live 通过。未执行生产部署、真实付费上游调用或数据库破坏性操作。

结论：阶段 01 的必需实现和失败路径已通过独立复核，标记为 `verified`。
