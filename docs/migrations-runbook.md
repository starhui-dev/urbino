# Urbino 数据库迁移操作

阶段 02 只提供存储基础，不表示计费、鉴权或网关请求链路已经完成。数据库是 PostgreSQL 18.x；应用使用 pgx，SQL 由 sqlc 生成，迁移使用 goose。迁移以增加结构为主，没有自动执行的 down。

## 角色与目标

由数据库管理员为一个明确的数据库配置独立的 migrator 和 runtime 登录角色。migrator 拥有 Urbino schema；runtime 仅取得迁移声明的运行权限，不应是超级用户、schema/database owner 或其他高权限角色的成员。不要把 migrator DSN 放进应用配置。

执行前核对数据库名称、环境、备份及恢复步骤。迁移命令只读取显式指定的 DSN 文件，并要求再次声明预期数据库名称及环境；不会从应用 DSN 自动推断迁移权限。

```powershell
go run ./cmd/urbino migrate --environment development --database urbino_test --dsn-file C:\explicit-test-secrets\migrator-dsn.txt
```

该路径是操作示例，不是仓库提供的凭据。DSN 文件由操作者提供，内容不得进入版本控制、命令行参数或日志。生产迁移应由部署流程在备份和兼容性检查后单次显式执行。并行执行通过 PostgreSQL advisory lock 串行化，重复执行不得重复创建对象。

## 应用启动与升级

`urbino serve` 在配置数据库连接时先连接并检查 schema 兼容性，再打开健康监听。应用启动不会运行 migration。未初始化、版本过新/过旧或数据库不可达时启动失败；开发环境未配置数据库时仍可运行已有的纯健康探针切片。

升级采用 expand/contract：先新增兼容字段、部署兼容读写版本，再另行安排数据迁移和清理。已有 migration 不修改；新结构使用新的版本文件。回滚首先回滚到仍兼容当前 schema 的应用版本，不用虚假的 down 删除账本。价格发布后不可变，账本更正新增分录，不改历史。

## 验证

日常检查：`go fmt ./...`、`go vet ./...`、`go test -count=1 ./...`、`go build ./cmd/urbino`。生成代码必须使用锁定的 sqlc 版本重新生成并检查差异。

真实 PostgreSQL 集成测试必须使用明确的本地隔离测试数据库。测试 helper 拒绝非 loopback、缺少测试数据库命名标记或不安全参数的 DSN；集成门禁缺失 DSN 或服务时应失败，不能当成通过。测试会写入测试结构和合成业务记录，不得指向生产或个人业务库。Docker/PG 不可用时继续执行单元、编译和静态检查，保留集成未运行证据。
