# 阶段 00 实现任务报告

- task/session：FoundationImplementer / urbino-implementer
- 目标：命令、配置路径、内部 health、版本切片
- 实际修改：`cmd/urbino/main.go`、`internal/cli/`、`internal/config/`、`internal/httpapi/`、`internal/version/` 及包内单测
- 关键契约：`version` 成功；未知命令退出 2；`--config > URBINO_CONFIG > ./urbino.yaml`；health 默认 `127.0.0.1:9091`，仅 `GET /healthz`
- 子任务限定验证：临时模块 `go build ./cmd/urbino`、`go vet ./...`、`go test ./...` 均 exit 0；真实二进制 version/unknown/missing-config/health/SIGTERM smoke 有结果
- 未验证：项目真实 go.mod 接入前的项目级命令；由主 Agent完成
- 任务终态：done；不代表阶段 verified
