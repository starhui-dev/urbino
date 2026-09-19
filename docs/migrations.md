# Urbino PostgreSQL 迁移运行手册

阶段 02 只提供显式迁移和 schema 兼容性边界。应用启动不自动创建或升级数据库；`urbino migrate` 必须由独立 migrator 身份执行。

## 角色边界

生产数据库应创建独立的 `urbino_migrator` 与 `urbino_app` 角色。migrator 才能创建/修改迁移对象；app 仅获得已发布表的最小 DML 权限，不能拥有 `CREATE`、`ALTER`、`DROP` 或直接修改不可变 journal 表的权限。实际角色、密码和 DSN 由部署系统安全注入，不写入配置模板、命令行或日志。

示例授权必须由数据库管理员在目标环境审阅后执行，不能直接复制到生产：

```sql
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE TEMP ON DATABASE <database_name> FROM PUBLIC;
REVOKE TEMP ON DATABASE <database_name> FROM urbino_app;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM urbino_app;
-- 按已启用阶段逐表 GRANT SELECT/INSERT/UPDATE，账本历史表不授予 UPDATE/DELETE。
```

## 执行

```text
urbino --config /etc/urbino/urbino.yaml migrate
```

配置只保存受限 `database.dsn_file` 引用；文件内容由部署系统提供。迁移 runner 使用 PostgreSQL advisory lock，按版本顺序在短事务内执行，并记录 `urbino_schema_migrations`。重复执行安全；并发执行者只有一个实际执行迁移。
读取 DSN 时拒绝符号链接、目录、特殊文件、非当前有效用户拥有的文件以及任意 group/other 权限；通过同一打开句柄完成 `fstat` 与读取。

二进制内置迁移资源，`migrate` 与带 database 配置的 `serve` 不依赖当前工作目录或仓库中的 `migrations/` 文件。根目录 SQL 仅作为可审阅源，`make generate-check` 会校验两份资源一致。

迁移历史保存版本、名称和 SQL SHA-256 checksum；已应用历史必须是当前迁移列表的连续前缀，名称或 checksum 漂移、缺口和未来版本均拒绝，且不自动修改数据库结构。

## 兼容性

应用只执行 `CheckCompatibility`，要求数据库版本等于应用声明的 expected version。版本不匹配时拒绝启动，不自动执行有损迁移。升级采用 expand/contract：先增加兼容结构，再切换读写，最后在独立窗口清理旧结构。

## 测试边界

阶段二的真实 PostgreSQL 测试必须使用临时、明确的 test DSN，禁止默认连接本机或生产数据库。没有可验证的 PostgreSQL 服务时，P02-T01..T06 记录为 NOT_RUN/BLOCKED；不能用 SQLite、fake pool 或 `go test` 通过代替数据库证据。
