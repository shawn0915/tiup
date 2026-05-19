# tiup sql — 变更记录

## 概述

为 TiUP 新增 `sql` 组件，提供交互式和批量 SQL 执行能力，支持连接 MySQL/TiDB 实例。

## 变更文件

### components/sql/

| 文件 | 说明 |
|------|------|
| `main.go` | 入口 |
| `cmd/root.go` | Cobra 命令定义，flag 绑定（连接、TLS、Playground 发现、连接管理、批量执行、输出格式、日志） |
| `cmd/terminal_unix.go` | Unix 密码输入（禁用回显） |
| `cmd/terminal_windows.go` | Windows 密码输入 |
| `connect/dsn.go` | DSN 解析（URL 格式、user:pass@host:port、host:port）、MySQL DSN 构造 |
| `connect/pool.go` | 连接管理，DB 接口抽象 |
| `connect/tls.go` | TLS 配置（CA 证书、客户端证书、skip-verify） |
| `connect/playground.go` | Playground 实例自动发现 |
| `connect/config.go` | 连接配置文件管理（`~/.tiup/connections.yaml`） |
| `format/formatter.go` | Formatter 接口定义 |
| `format/table.go` | 表格输出 |
| `format/csv.go` | CSV 输出 |
| `format/json.go` | JSON / JSON-pretty / JSON-rows 输出 |
| `format/tsv.go` | TSV 输出 |
| `format/vertical.go` | 垂直表格输出（MySQL `\G` 风格） |
| `repl/repl.go` | 交互式 REPL（元命令 `\q` `\l` `\d` `\u` `\s` `\t` `\G` `\T` `\h` `\timing` `\i`） |
| `batch/executor.go` | 批量执行器（文件、目录、stdin、错误策略） |
| `log/logger.go` | 查询日志（文件输出、DSN 脱敏） |

### 测试文件

| 文件 | 说明 |
|------|------|
| `connect/dsn_test.go` | DSN 解析各种格式 |
| `connect/config_test.go` | 连接配置 CRUD |
| `connect/tls_test.go` | TLS 配置（禁用、skip-verify、缺失证书） |
| `format/formatter_test.go` | 所有格式输出验证 |
| `batch/executor_test.go` | SQL 分割和读取（含字符串内分号） |
| `log/logger_test.go` | 日志记录和脱敏 |
| `cmd/root_test.go` | Cobra 命令定义、flag 默认值 |
| `repl/repl_test.go` | REPL 元命令、标识符校验、格式切换、慢查询阈值 |

## 修复历史

### 第一轮修复（审阅人：@元芳）

| 编号 | 严重度 | 文件 | 修复内容 |
|------|--------|------|----------|
| C1 | 阻断 | `connect/dsn.go` | 移除 `tlsConfigName()` 内联实现（缺少 import），改为 `tlsDSNParam()` 仅返回配置名 |
| C2 | 阻断 | `connect/tls.go`, `cmd/root.go` | 使用 `mysql.RegisterTLSConfig()` 实际注册 TLS 配置；`root.go` 连接前调用 `SetupTLS()` |
| H1 | 高 | `repl/repl.go` | `\d`、`\u` 元命令增加 `isValidIdentifier()` 校验 + 反引号引用，防止 SQL 注入 |
| H2 | 高 | `cmd/root.go` | `--password/-p` 改为布尔标志，仅触发交互式输入；密码来源优先级：DSN → 环境变量 → 交互式 |
| H3 | 高 | `connect/dsn.go` | `ToMySQLDSN()` 对用户名和密码使用 `url.QueryEscape()` URL 编码 |
| H4 | 高 | `cmd/terminal_unix.go` | 密码输入退格键改为只删除最后一个字符（`runes` 切片） |

### 第二轮修复（审阅人：@全能表妹）

| 编号 | 严重度 | 文件 | 修复内容 |
|------|--------|------|----------|
| M1 | 高 | `batch/executor.go` | `splitStatements()` 和 `readStatements()` 增加单引号/双引号内分号感知，避免错误分割 SQL 字符串内的分号 |
| M2 | 中 | `batch/executor.go` | 执行计数器 `successes`/`failures` 从结构体字段改为 `execStatements()` 局部变量，避免跨调用累积 |
| M3 | 中 | `format/vertical.go` | `maxLabelLen` 计算从列内循环提升到行循环顶层，消除 O(N²) 重复计算 |
| M4 | 中 | `connect/config_test.go` | `setTestHome()` 使用 `os.LookupEnv` + `os.Unsetenv` 正确恢复环境变量 |

### 第三轮修复（审阅人：@元芳）

| 编号 | 严重度 | 文件 | 修复内容 |
|------|--------|------|----------|
| N1 | 中 | `format/vertical.go` | `maxLabelLen` 从行循环内提升到行循环外，仅计算一次 |

### 编译验证与补测

- `go build ./components/sql/` 编译通过
- 新增测试文件：`connect/tls_test.go`（5 用例）、`cmd/root_test.go`（3 用例）、`repl/repl_test.go`（13 用例）
- 全部 **56 个测试用例** 通过，覆盖 6 个包

## 安全说明

> **⚠️ S1：TIUP_SQL_PASSWORD 环境变量明文存储风险**
>
> `TIUP_SQL_PASSWORD` 环境变量以明文形式存储在进程环境中。同一主机的其他进程可能通过 `/proc/<pid>/environ`（Linux）或类似机制读取该值。
>
> **建议**：
> - 仅在可信的单用户开发环境中使用环境变量传递密码
> - 生产环境推荐使用 TiUP 的连接配置文件（`tiup sql --connection <name>`）或交互式密码输入
> - 连接配置文件以 `0o600` 权限存储，仅限当前用户可读
> - 避免在 CI/CD 脚本中直接设置 `TIUP_SQL_PASSWORD`，应使用密钥管理服务
