# 阶段 00 · 独立复核

复核 revision：HEAD `14ee4c3a0fa851457af62586302a2de6866f6478` 加工作树修复。

复核了 `cmd/urbino/main.go`、`cmd/urbino/main_test.go`、clock/securitylog、配置样例、Makefile、CI、依赖锁定和阶段证据。发现一个 P1：`serve` 原先在 `ListenAndServe` 绑定失败后只记录日志，仍输出 serving 并等待信号，导致启动失败被伪装为成功。已改为先 `net.Listen`，绑定失败立即返回；服务运行使用 `serveHealth`，监听错误返回，取消时执行有界 shutdown，并补充无效地址、取消关闭、监听失败测试。

复核命令及结果见 `evidence/raw/review-00.txt`：绝对路径 Go 1.27.1 的 `go test -count=1 ./...`、`go vet ./...`、`go build -o urbino.exe ./cmd/urbino`、`go list -m all` 均 exit 0；运行二进制后 `/healthz` 返回 200，模型路径仍为 404。模块列表无 CPA/S2A。随后运行 `check_evidence.py 00 --require-review` exit 0。

证据结构检查最初发现 P00-T02/P00-T03 使用分号拼接的 `source_path`，工具无法解析；已分别改为实际存在的 `go.mod` 与 `configs/urbino.example.yaml`，重新检查通过。

输入缺失：仓库没有用户要求的 `project.json`，也没有 `docs/14-naming.md`；不能据此补造内容。`make`、`docker`、默认 PATH 的 `go`/`python` 当前不可用，但等价 Go 检查已实际执行，未将未运行工具冒充通过。

补充检查：`go test -race` 受环境限制未通过（默认 CGO_DISABLED；启用 CGO 后缺少 gcc）。该检查不属于阶段 00 专属必需门禁，已如实记录，留待后续具备 C 编译器的环境执行。

`tools/test_tools.py` 在当前 Python 3.14/Windows 控制台编码下因子进程中文输出解码异常失败；`validate_pack.py` 与 `check_evidence.py` 已分别独立运行并通过，未把该工具自测失败伪装为通过。

结论：阶段 00 必需代码、失败路径和回归检查通过，标记 `verified`。后续阶段接入未使用依赖时需重新核验版本与许可证。
