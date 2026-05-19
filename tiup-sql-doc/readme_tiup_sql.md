# tiup sql 组件使用与开发文档

> **版本**: 基于 TiUP 主仓库 `components/sql/` 目录  
> **目标**: 为 TiUP 提供通用 SQL 客户端能力，支持 MySQL 与 TiDB  
> **状态**: 功能已补全，测试全部通过，已在真实 MySQL 实例验证

---

## 1. 设计思路

### 1.1 背景

TiUP 是 PingCAP 生态的组件管理 CLI，原有 `tiup client` 仅能连接本地 playground，不支持远程数据库、SQL 批量执行、结果导出等高级能力。

### 1.2 目标

为 TiUP 新增 `tiup sql` 组件，提供：

- 通过 DSN/连接字符串直连任意 MySQL 或 TiDB 实例
- 自动发现本地 playground 实例（兼容 `tiup client`）
- 交互式 SQL REPL（Read-Eval-Print Loop）
- SQL 文件批量执行与 stdin 管道执行
- 查询结果多格式输出（表格、CSV、JSON、TSV、垂直表格）
- 安全的连接管理与凭据处理
- 连接历史与常用连接收藏

### 1.3 架构设计

```
┌─────────────────────────────────────────────────────────┐
│                    tiup sql                              │
├─────────────┬──────────────┬─────────────┬──────────────┤
│   连接管理   │  交互式 REPL  │  批量执行    │  输出格式化   │
│  (connect)  │   (repl)     │  (batch)    │  (format)    │
├─────────────┼──────────────┼─────────────┼──────────────┤
│ • DSN 解析   │ • 行编辑     │ • 文件执行   │ • 表格输出   │
│ • 连接池管理  │ • 历史记录   │ • stdin 管道 │ • CSV 输出   │
│ • TLS 支持   │ • 多行编辑   │ • 事务控制   │ • JSON 输出  │
│ • 凭据安全   │ • 特殊命令   │ • 错误处理   │ • TSV 输出   │
│ • Playground │              │ • 进度显示   │ • 垂直表格   │
│   实例发现   │              │             │              │
├─────────────┴──────────────┴─────────────┴──────────────┤
│              go-sql-driver/mysql (MySQL/TiDB 驱动)        │
└─────────────────────────────────────────────────────────┘
```

### 1.4 核心类型

```go
// DSN 配置
type DSNConfig struct {
    Host         string
    Port         int
    User         string
    Password     string
    Database     string
    Protocol     string
    Socket       string
    TLSConfig    *TLSConfig
    Timeout      time.Duration
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    Charset      string
    Collation    string
    Params       map[string]string
}

// 格式化器接口
type Formatter interface {
    FormatResult(w io.Writer, columns []string, rows [][]any, rowCount int, duration time.Duration) error
    FormatError(w io.Writer, err error) error
    Flush() error
    SetWriter(w io.Writer)
}
```

---

## 2. 开发模式与项目结构

```
components/sql/
├── main.go                  # 入口：cobra 命令注册
├── cmd/
│   ├── root.go              # 根命令定义与 flag 绑定
│   ├── root_test.go         # 命令 flag 与配置逻辑测试
│   ├── terminal_unix.go     # Unix 终端 raw 模式
│   └── terminal_windows.go  # Windows 终端 raw 模式
├── connect/
│   ├── dsn.go               # DSN 解析与构造
│   ├── dsn_test.go          # DSN 解析测试
│   ├── pool.go              # 连接池管理（DB 接口抽象）
│   ├── pool_test.go         # 连接池测试
│   ├── tls.go               # TLS 配置与验证
│   ├── tls_test.go          # TLS 测试
│   ├── config.go            # 连接配置文件读写
│   ├── config_test.go       # 配置 CRUD 测试
│   └── playground.go        # Playground 实例发现
├── repl/
│   ├── repl.go              # REPL 主循环与元命令
│   └── repl_test.go         # REPL 与元命令测试
├── batch/
│   ├── executor.go          # 批量执行器
│   └── executor_test.go     # 批量执行测试
├── format/
│   ├── formatter.go         # 格式化器接口与工厂
│   ├── formatter_test.go    # 格式化器综合测试
│   ├── table.go             # 表格格式
│   ├── csv.go               # CSV 格式
│   ├── json.go              # JSON 格式（含 pretty/rows）
│   ├── tsv.go               # TSV 格式
│   └── vertical.go          # 垂直表格格式
└── log/
    ├── logger.go            # 查询日志记录
    └── logger_test.go       # 日志与脱敏测试
```

### 开发原则

- **最小改动**：优先复用现有接口，不破坏已有行为
- **接口抽象**：`connect.DB` 接口封装 `*sql.DB`，便于 mock 测试
- **安全优先**：密码交互式输入、TLS 默认验证、日志脱敏
- **引号感知**：SQL 分割器正确处理字符串内的分号与转义引号

---

## 3. 命令示例

### 3.1 连接方式

```bash
# URL 格式
tiup sql mysql://root:password@192.168.1.101:3308/test_db

# 键值对参数
tiup sql --host 192.168.1.101 --port 3308 --user root test_db

# 仅指定主机（交互式输入密码）
tiup sql 192.168.1.101:3308

# 使用已保存的连接
tiup sql -c local-dev

# 自动发现本地 playground
tiup sql --playground
```

### 3.2 执行模式

```bash
# 执行单条 SQL 后退出
tiup sql -e "SHOW DATABASES" mysql://root:pw@host:3308/

# 执行 SQL 文件
tiup sql -f init.sql mysql://root:pw@host:3308/db

# 执行目录下所有 .sql 文件（按文件名排序）
tiup sql -f ./migrations/ mysql://root:pw@host:3308/db

# 管道执行
echo "SELECT 1" | tiup sql mysql://root:pw@host:3308/

# heredoc 执行
tiup sql mysql://root:pw@host:3308/ <<EOF
CREATE DATABASE IF NOT EXISTS test;
USE test;
EOF
```

### 3.3 输出格式

```bash
# CSV（默认带表头）
tiup sql --format csv -e "SELECT * FROM users" mysql://root:pw@host:3308/ > users.csv

# JSON 数组
tiup sql --format json -e "SELECT * FROM users" mysql://root:pw@host:3308/

# 美化 JSON
tiup sql --format json-pretty -e "SELECT * FROM users" mysql://root:pw@host:3308/

# NDJSON（每行一个对象）
tiup sql --format json-rows -e "SELECT * FROM users" mysql://root:pw@host:3308/

# TSV
tiup sql --format tsv -e "SELECT * FROM users" mysql://root:pw@host:3308/

# 垂直表格（适合宽表）
tiup sql --format vertical -e "SELECT * FROM users WHERE id = 1" mysql://root:pw@host:3308/

# 无表头 CSV
tiup sql --no-header --format csv -e "SELECT 1" mysql://root:pw@host:3308/
```

### 3.4 批量执行选项

```bash
# 自定义语句分隔符
tiup sql --delimiter "GO" -e "SELECT 1 GO SELECT 2 GO" mysql://root:pw@host:3308/

# 遇到错误继续执行
tiup sql --on-error continue -f script.sql mysql://root:pw@host:3308/

# 仅解析不执行（dry-run）
tiup sql --dry-run -f script.sql mysql://root:pw@host:3308/

# 执行前回显 SQL
tiup sql --echo -f script.sql mysql://root:pw@host:3308/
```

### 3.5 连接管理

```bash
# 保存当前连接配置
tiup sql --save-connection prod --host 10.0.1.100 --port 3306 --user app mysql://root:pw@host:3308/

# 列出所有保存的连接
tiup sql --list-connections

# 删除连接配置
tiup sql --delete-connection prod
```

### 3.6 其他选项

```bash
# 启用 TLS
tiup sql --tls --tls-ca /path/to/ca.pem mysql://root@host:3308/

# 跳过证书验证（仅开发环境）
tiup sql --tls-skip-verify mysql://root@host:3308/

# 记录查询日志
tiup sql --log /path/query.log -e "SELECT 1" mysql://root:pw@host:3308/

# 关闭计时显示
tiup sql --timing=false -e "SELECT 1" mysql://root:pw@host:3308/

# 使用环境变量传递密码
export TIUP_SQL_PASSWORD=secret
tiup sql -e "SELECT 1" mysql://root@host:3308/
```

### 3.7 REPL 元命令

进入交互式模式（不指定 `-e`、`-f` 且 stdin 为 TTY）：

```
mysql> \l                    # 列出所有数据库
mysql> \d                    # 列出当前数据库的所有表
mysql> \d users              # 显示表结构
mysql> \u test_db            # 切换数据库
mysql> \t csv                # 切换输出格式
mysql> \G                    # 切换为垂直显示
mysql> \T                    # 切换为表格显示
mysql> \timing               # 开关查询计时
mysql> \i script.sql         # 执行 SQL 文件
mysql> \echo hello           # 输出文本
mysql> \s                    # 显示连接状态
mysql> \h                    # 显示帮助
mysql> \q                    # 退出
```

---

## 4. 用法与场景

| 场景 | 推荐用法 |
|------|---------|
| **快速查询** | `tiup sql -e "SELECT ..." mysql://...` |
| **数据导出** | `tiup sql --format csv -e "SELECT ..." > out.csv` |
| **批量迁移** | `tiup sql -f ./migrations/ mysql://...` |
| **CI/CD 脚本** | `tiup sql --on-error stop --echo -f schema.sql mysql://...` |
| **日常运维** | 保存连接后使用 `tiup sql -c prod` 进入交互式 REPL |
| **敏感环境** | 使用 `--password` 交互输入，或环境变量 `TIUP_SQL_PASSWORD` |
| **TLS 生产环境** | `tiup sql --tls --tls-ca ca.pem --tls-cert client.pem --tls-key key.pem mysql://...` |

---

## 5. 测试用例

### 5.1 测试覆盖概览

| 包 | 测试文件 | 核心覆盖点 | 用例数 |
|----|---------|-----------|--------|
| `cmd` | `root_test.go` | 命令创建、flag 默认值、配置覆盖逻辑、TLS 配置应用 | 7 |
| `connect` | `dsn_test.go` | URL/键值对/主机格式解析、参数解析、IPv6、编码、脱敏 | 16 |
| `connect` | `config_test.go` | 保存/加载/删除/列出连接、非法名称、文件创建 | 4 |
| `connect` | `pool_test.go` | 接口编译检查、无效 DSN 错误 | 3 |
| `connect` | `tls_test.go` | TLS 开关、跳过验证、缺失证书、安全名称过滤 | 8 |
| `batch` | `executor_test.go` | 语句分割、引号感知、文件/stdin 执行、DML/Query 路径、DryRun、错误策略、MaxRows、目录执行 | 16 |
| `repl` | `repl_test.go` | 标识符校验/转义、REPL 创建、查询读取、元命令、DML/Query 区分、状态输出 | 24 |
| `format` | `formatter_test.go` | 表格/CSV/JSON/TSV/垂直格式输出、nil/time/binary、空结果、无头模式、工厂方法 | 18 |
| `log` | `logger_test.go` | 日志禁用、文件写入、DSN 脱敏 | 3 |

**合计: 90+ 测试用例**

### 5.2 关键测试说明

#### DSN 解析测试
- `TestParseDSN_URLFormat`：验证 `mysql://user:pass@host:port/db` 完整解析
- `TestParseDSN_URLWithParams`：验证 `charset`、`timeout`、`tls=skip-verify` 等 URL 参数
- `TestParseDSN_IPV6`：验证 `[::1]:4000` IPv6 格式
- `TestToMySQLDSN_SpecialChars`：验证密码中的 `@`、`:`、`/` 被正确 URL 编码
- `TestSanitizedDSN`：验证 DSN 日志输出时密码被替换为 `***`

#### 批量执行测试
- `TestSplitStatements_SemicolonsInStrings`：验证单引号、双引号、转义引号内的分号**不会**错误分割语句
- `TestExecutor_ExecString_DML`：验证 INSERT/UPDATE 走 `Exec()` 路径，输出 `Query OK, n row(s) affected`
- `TestExecutor_ExecString_OnErrorStop/Continue`：验证 `--on-error stop` 立即停止，`continue` 记录警告并继续
- `TestExecutor_ExecString_DryRun`：验证 `--dry-run` 仅打印不执行
- `TestExecutor_ExecString_MaxRows`：验证 MaxRows 选项已接入执行器

#### REPL 测试
- `TestHandleMetaCommand_*`：覆盖 `\q` `\l` `\d` `\u` `\t` `\G` `\T` `\timing` `\echo` `\s` `\c`
- `TestREPL_execDML`：验证 REPL 中执行 INSERT 走 `Exec()` 而非 `Query()`
- `TestIsValidIdentifier`：验证 SQL 注入防护（拒绝 `;` ` ` `` ` 等非法字符）

#### 格式化测试
- `TestTableFormatter_BinaryValue`：验证非 UTF-8 `[]byte` 显示为 `<BINARY n bytes>`
- `TestTableFormatter_TimeValue`：验证 `time.Time` 格式化为 `2006-01-02 15:04:05`
- `TestJSONFormatter_NilValue`：验证 JSON 中 `nil` 映射为 `null`
- `TestVerticalFormatter_EmptyResult`：验证空结果集输出 `Empty set`

---

## 6. 测试结果

### 6.1 全部测试通过

```bash
$ cd /data/tiup-sql
$ go test ./components/sql/... -count=1
?       github.com/pingcap/tiup/components/sql        [no test files]
ok      github.com/pingcap/tiup/components/sql/batch  0.005s
ok      github.com/pingcap/tiup/components/sql/cmd    0.005s
ok      github.com/pingcap/tiup/components/sql/connect 0.011s
ok      github.com/pingcap/tiup/components/sql/format 0.006s
ok      github.com/pingcap/tiup/components/sql/log    0.006s
ok      github.com/pingcap/tiup/components/sql/repl   0.007s
```

### 6.2 构建验证

```bash
$ go build -o bin/tiup-sql ./components/sql/
# 构建成功，无错误
```

### 6.3 真实 MySQL 实例验证

在 **192.168.1.101:3308**（user: `root`, password: `Shawn2026`）上实际操作并验证通过的功能：

| 验证项 | 状态 |
|--------|------|
| URL 格式连接 | ✅ |
| 键值对参数连接 | ✅ |
| 保存连接（`-c`） | ✅ |
| 仅指定主机 | ✅ |
| 全部输出格式（table/csv/json/json-pretty/json-rows/tsv/vertical） | ✅ |
| SQL 文件执行（`-f`） | ✅ |
| 目录批量执行 | ✅ |
| 管道/stdin 执行 | ✅ |
| DML 操作（INSERT/UPDATE/DELETE） | ✅ |
| dry-run 模式 | ✅ |
| on-error continue/stop | ✅ |
| echo 模式 | ✅ |
| 查询日志（`--log`） | ✅ |
| no-header | ✅ |
| 自定义 delimiter | ✅ |
| 连接管理（save/list/delete） | ✅ |
| 环境变量密码 | ✅ |
| 帮助信息 | ✅ |

---

## 7. 安全与合规

| 检查项 | 状态 |
|--------|------|
| 密码不以命令行明文传递（`-p` 为 bool flag） | ✅ |
| 密码交互式密文输入 | ✅ |
| TLS 证书验证默认启用 | ✅ |
| `--tls-skip-verify` 需显式指定 | ✅ |
| 元命令 SQL 注入防护（`isValidIdentifier` + 反引号包裹） | ✅ |
| 日志中密码脱敏（`SanitizedDSN`） | ✅ |
| 连接配置文件权限 `0o600` | ✅ |
| 连接名称安全检查 | ✅ |

---

## 8. 快速开始

```bash
# 1. 构建
cd /data/tiup-sql
go build -o bin/tiup-sql ./components/sql/

# 2. 连接本地或远程 MySQL/TiDB
export TIUP_SQL_PASSWORD=your_password
./bin/tiup-sql -e "SHOW DATABASES" mysql://root@127.0.0.1:3306/

# 3. 运行测试
go test ./components/sql/... -v
```

---

*本文档由开发团队维护，如有更新请以最新源码为准。*
