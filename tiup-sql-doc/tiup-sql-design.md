# tiup sql 组件设计文档

## 1. 概述

### 1.1 背景

TiUP 是 PingCAP 生态系统的组件管理 CLI 工具，目前已有 `tiup client` 组件可连接本地 playground 实例，但功能有限：仅支持本地 playground 实例发现，不支持直接指定远程数据库地址，不支持连接 MySQL，且缺乏 SQL 执行、结果导出等高级功能。

### 1.2 目标

为 TiUP 新增 `tiup sql` 组件，提供通用 SQL 客户端能力，支持连接 **MySQL** 和 **TiDB**，具备以下核心能力：

- 直接通过 DSN/连接字符串连接任意 MySQL 或 TiDB 实例
- 自动发现并连接本地 playground 实例（兼容现有 `tiup client` 能力）
- 交互式 SQL REPL（Read-Eval-Print Loop）
- SQL 文件批量执行
- 查询结果多格式输出（表格、CSV、JSON）
- 安全的连接管理和凭据处理
- 连接历史和常用连接收藏

### 1.3 与现有组件的关系

| 组件 | 定位 | 与 `tiup sql` 的关系 |
|------|------|---------------------|
| `tiup client` | 仅连接本地 playground | `tiup sql` 完全替代并增强 |
| `tiup playground` | 本地集群启动 | `tiup sql` 可自动发现其启动的实例 |
| `tiup cluster` | 远程集群管理 | `tiup sql` 可利用 cluster 的拓扑信息简化连接 |

---

## 2. 功能模块设计

### 2.1 模块总览

```
┌─────────────────────────────────────────────────────────┐
│                    tiup sql                              │
├─────────────┬──────────────┬─────────────┬──────────────┤
│   连接管理   │  交互式 REPL  │  批量执行    │  输出格式化   │
│  (connect)  │   (repl)     │  (batch)    │  (format)    │
├─────────────┼──────────────┼─────────────┼──────────────┤
│ • DSN 解析   │ • 行编辑     │ • 文件执行   │ • 表格输出   │
│ • 连接池管理  │ • 自动补全   │ • stdin 管道 │ • CSV 输出   │
│ • TLS 支持   │ • 语法高亮   │ • 事务控制   │ • JSON 输出  │
│ • 凭据安全   │ • 历史记录   │ • 错误处理   │ • TSV 输出   │
│ • Playground │ • 多行编辑   │ • 进度显示   │ • 垂直表格   │
│   实例发现   │ • 特殊命令   │             │              │
├─────────────┴──────────────┴─────────────┴──────────────┤
│                    核心引擎 (usql)                        │
├─────────────────────────────────────────────────────────┤
│              go-sql-driver/mysql (MySQL/TiDB 驱动)        │
└─────────────────────────────────────────────────────────┘
```

### 2.2 模块一：连接管理 (connect)

#### 2.2.1 DSN 解析与构造

支持多种连接指定方式：

```bash
# 方式一：URL 格式
tiup sql mysql://root:password@127.0.0.1:4000/test_db

# 方式二：键值对参数
tiup sql --host 127.0.0.1 --port 4000 --user root --password test_db

# 方式三：仅指定主机（交互式输入密码）
tiup sql 127.0.0.1:4000

# 方式四：连接本地 playground
tiup sql --playground

# 方式五：通过 cluster 拓扑连接
tiup sql --cluster my-cluster --component tidb
```

**DSN 解析器** 将以上各种输入统一转换为标准 MySQL DSN 格式：

```
[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
```

支持的关键 DSN 参数：
| 参数 | 默认值 | 说明 |
|------|--------|------|
| `tls` | `false` | TLS 连接配置 |
| `timeout` | `5s` | 连接超时 |
| `readTimeout` | `30s` | 读超时 |
| `writeTimeout` | `30s` | 写超时 |
| `maxAllowedPacket` | `64M` | 最大数据包 |
| `charset` | `utf8mb4` | 字符集 |
| `collation` | `utf8mb4_general_ci` | 排序规则 |

#### 2.2.2 TLS/SSL 支持

```bash
# 跳过证书验证（仅用于开发环境）
tiup sql --tls-skip-verify mysql://root@host:4000/

# 指定 CA 证书
tiup sql --tls-ca /path/to/ca.pem mysql://root@host:4000/

# 完整 TLS 配置
tiup sql --tls-ca /path/to/ca.pem \
         --tls-cert /path/to/client-cert.pem \
         --tls-key /path/to/client-key.pem \
         mysql://root@host:4000/
```

#### 2.2.3 Playground 实例自动发现

复用现有 `tiup client` 的实例发现逻辑，扫描 `~/.tiup/data/` 下的实例元数据和 DSN 文件：

```go
func discoverPlaygroundEndpoints(tiupHome string) ([]*Endpoint, error) {
    // 1. 读取 data 目录下所有实例
    // 2. 通过 gopsutil 检查进程是否存活
    // 3. 读取每个实例的 dsn 文件
    // 4. 返回可用端点列表
}
```

#### 2.2.4 凭据安全管理

- **密码输入**：交互模式下使用终端密文输入（`golang.org/x/term` 的 `ReadPassword`），不在命令行参数中暴露密码
- **连接配置文件**：可选的 `~/.tiup/connections.yaml` 存储常用连接（密码使用 `golang.org/x/crypto/bcrypt` 哈希存储，运行时解密由 OS 密钥链管理）
- **环境变量**：支持 `TIUP_SQL_PASSWORD` 环境变量传递密码（优先级低于交互输入）
- **连接配置文件格式**：

```yaml
connections:
  - name: local-dev
    host: 127.0.0.1
    port: 4000
    user: root
    database: test
    tls: false
  - name: prod-tidb
    host: 10.0.1.100
    port: 4000
    user: app_user
    # password: 从交互式输入或密钥链获取
    tls:
      ca: /etc/ssl/tidb-ca.pem
      cert: /etc/ssl/client-cert.pem
      key: /etc/ssl/client-key.pem
```

使用方式：

```bash
# 使用已保存的连接
tiup sql -c local-dev

# 管理连接配置
tiup sql --save-connection local-dev mysql://root@127.0.0.1:4000/test
tiup sql --list-connections
tiup sql --delete-connection prod-tidb
```

### 2.3 模块二：交互式 REPL (repl)

#### 2.3.1 行编辑器

基于 [usql](https://github.com/xo/usql) 的 `rline` 包提供：

- 行编辑：Emacs/Vi 键绑定
- 历史记录：持久化到 `~/.tiup/sql/history`（每个连接独立历史）
- 多行编辑：分号结尾的语句自动执行，支持反斜杠续行
- 自动补全（详见 2.3.2）

#### 2.3.2 自动补全

| 补全类型 | 示例 |
|----------|------|
| SQL 关键字 | `SEL` → `SELECT` |
| 数据库名 | `USE ` → 列出所有数据库 |
| 表名 | `SELECT * FROM ` → 列出当前数据库的表 |
| 列名 | `SELECT t.` → 列出表 `t` 的所有列 |
| 特殊命令 | `\` → 列出所有 `\` 开头的元命令 |

实现方式：在用户输入空格或 `.` 后触发补全请求，通过 `SHOW DATABASES`、`SHOW TABLES`、`DESCRIBE <table>` 等元数据查询获取候选列表。

#### 2.3.3 元命令 (Meta Commands)

以反斜杠 `\` 开头的内置命令，不发送到数据库：

| 命令 | 别名 | 说明 |
|------|------|------|
| `\q` | `\exit`, `\quit` | 退出 |
| `\c` | `\connect` | 重新连接 |
| `\d` | | 列出当前数据库的所有表 |
| `\d <table>` | | 显示表结构 |
| `\l` | | 列出所有数据库 |
| `\u <db>` | `\use` | 切换数据库 |
| `\h` | `\help` | 显示帮助 |
| `\s` | `\status` | 显示连接状态信息 |
| `\G` | | 切换结果为垂直显示模式 |
| `\T` | | 切换结果为表格显示模式 |
| `\W` | | 切换结果写入文件 `\w <file>` |
| `\t <format>` | | 切换输出格式（table/csv/json/tsv） |
| `\e` | `\edit` | 用外部编辑器编辑 SQL |
| `\i <file>` | `\include` | 执行 SQL 文件 |
| `\echo <text>` | | 输出文本 |
| `\timing` | | 切换查询计时显示 |
| `\pager [cmd]` | | 设置/取消分页器 |
| `\system <cmd>` | `\!` | 执行系统命令 |
| `\warnings` | | 切换警告显示 |

#### 2.3.4 会话变量管理

```sql
-- TiDB 特有的会话变量快捷设置
SET SESSION tidb_max_execution_time = 60000;
SET SESSION sql_mode = 'STRICT_TRANS_TABLES';

-- 显示当前会话变量
SHOW VARIABLES LIKE 'max_execution_time';
SHOW SESSION VARIABLES;
```

### 2.4 模块三：批量执行 (batch)

#### 2.4.1 文件执行

```bash
# 执行 SQL 文件
tiup sql mysql://root@127.0.0.1:4000/test -f init.sql

# 执行多个 SQL 文件
tiup sql mysql://root@127.0.0.1:4000/ -f schema.sql -f data.sql

# 执行目录下所有 .sql 文件（按文件名排序）
tiup sql mysql://root@127.0.0.1:4000/ -f ./migrations/
```

执行规则：
- 以 `--` 开头的行为注释，跳过
- 空行跳过
- 以分号结尾的语句作为一条 SQL 执行
- 支持 `delimiter` 指令更改语句分隔符（兼容存储过程/函数定义）
- 遇到错误时的行为由 `--on-error` 参数控制

#### 2.4.2 stdin 管道执行

```bash
# 从管道执行
echo "SELECT 1" | tiup sql mysql://root@127.0.0.1:4000/

# 从其他命令的输出执行
mysqldump --no-data db | tiup sql mysql://root@127.0.0.1:4000/new_db

# heredoc 执行
tiup sql mysql://root@127.0.0.1:4000/ <<EOF
CREATE DATABASE IF NOT EXISTS test;
USE test;
CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY, name VARCHAR(100));
INSERT INTO users VALUES (1, 'alice');
EOF
```

#### 2.4.3 批量执行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--file` / `-f` | - | SQL 文件路径或目录 |
| `--on-error` | `stop` | 错误处理：`stop`（停止）、`continue`（继续）、`abort`（回滚并停止） |
| `--delimiter` | `;` | 语句分隔符 |
| `--echo` | `false` | 执行前回显 SQL |
| `--force` | `false` | 忽略连接错误继续执行 |
| `--max-rows` | `0`（无限制） | 结果集最大行数 |
| `--dry-run` | `false` | 仅解析不执行 |

### 2.5 模块四：输出格式化 (format)

#### 2.5.1 表格格式 (table)

默认格式，适合交互式查看：

```
+----+-------+---------------------+
| id | name  | created_at          |
+----+-------+---------------------+
|  1 | alice | 2026-05-17 10:00:00 |
|  2 | bob   | 2026-05-17 10:05:00 |
+----+-------+---------------------+
2 rows in set (0.01 sec)
```

#### 2.5.2 CSV 格式

```bash
tiup sql --format csv -e "SELECT * FROM users" > users.csv
```

输出带 BOM 的 UTF-8 CSV（Excel 兼容）：
```csv
id,name,created_at
1,"alice","2026-05-17 10:00:00"
2,"bob","2026-05-17 10:05:00"
```

#### 2.5.3 JSON 格式

```bash
tiup sql --format json -e "SELECT * FROM users"
```

输出：
```json
[
  {"id": 1, "name": "alice", "created_at": "2026-05-17 10:00:00"},
  {"id": 2, "name": "bob", "created_at": "2026-05-17 10:05:00"}
]
```

JSON 格式子选项：
- `json`：紧凑数组
- `json-pretty`：美化输出
- `json-rows`：每行一个 JSON 对象（NDJSON）

#### 2.5.4 TSV 格式

Tab 分隔，适合程序间传递：

```bash
tiup sql --format tsv -e "SELECT * FROM users"
```

#### 2.5.5 垂直表格格式

适合宽表查看：

```bash
tiup sql --format vertical -e "SELECT * FROM users WHERE id = 1"
```

输出：
```
*************************** 1. row ***************************
      id: 1
    name: alice
created_at: 2026-05-17 10:00:00
1 row in set (0.00 sec)
```

### 2.6 模块五：日志与诊断

#### 2.6.1 查询日志

可选地将所有执行的 SQL 记录到日志文件：

```bash
tiup sql --log /path/to/query.log mysql://root@127.0.0.1:4000/
```

日志格式：

```
[2026-05-17T10:00:00.000Z] [conn:1] [timing:12.5ms] [rows:2] SELECT * FROM users;
[2026-05-17T10:00:01.000Z] [conn:1] [timing:0.5ms] [rows:0] USE test;
```

#### 2.6.2 慢查询提示

当查询执行时间超过阈值（默认 1s，可通过 `--slow-threshold` 配置）时，自动高亮显示警告：

```
mysql> SELECT COUNT(*) FROM large_table;
+----------+
| COUNT(*) |
+----------+
| 10000000 |
+----------+
1 row in set (2.35 sec) ⚠ slow query (>1.00s)
```

---

## 3. 命令行接口设计

### 3.1 完整命令参数

```
Usage:
  tiup sql [flags] [DSN|connection-name]

Examples:
  tiup sql mysql://root@password@127.0.0.1:4000/test_db
  tiup sql --host 127.0.0.1 --port 4000 --user root test_db
  tiup sql -c local-dev

Flags:
  连接选项:
    -h, --host string          数据库主机地址 (default "127.0.0.1")
    -P, --port int             数据库端口 (default 4000)
    -u, --user string          用户名 (default "root")
    -p, --password string      密码（推荐交互式输入）
        --database string      默认数据库
        --socket string        Unix socket 路径
        --protocol string      连接协议：tcp, unix (default "tcp")

  TLS 选项:
        --tls                  启用 TLS
        --tls-ca string        CA 证书文件路径
        --tls-cert string      客户端证书文件路径
        --tls-key string       客户端私钥文件路径
        --tls-skip-verify      跳过证书验证

  Playground 发现:
        --playground           自动发现并连接本地 playground 实例
        --cluster string       通过 TiUP cluster 拓扑获取连接信息
        --component string     连接的组件类型，配合 --cluster 使用 (default "tidb")

  连接管理:
    -c, --connection string    使用已保存的连接配置名称
        --save-connection      保存当前连接到配置
        --list-connections     列出所有已保存的连接
        --delete-connection string  删除指定连接配置

  执行选项:
    -e, --execute string       执行 SQL 后退出（非交互模式）
    -f, --file strings         执行 SQL 文件（非交互模式）
        --delimiter string     语句分隔符 (default ";")
        --on-error string      错误处理策略：stop, continue, abort (default "stop")
        --dry-run              仅解析不执行
        --echo                 回显执行的 SQL
        --force                忽略连接错误

  输出选项:
        --format string        输出格式：table, csv, json, json-pretty, json-rows, tsv, vertical (default "table")
        --no-header            不显示列标题
        --separator string     CSV/TSV 分隔符
        --pager string         分页命令（如 "less -S"）
        --timing               显示查询执行时间 (default true)
        --slow-threshold duration  慢查询阈值 (default "1s")

  日志选项:
        --log string           SQL 查询日志文件路径

  通用选项:
        --help                 显示帮助信息
        --version              显示版本信息
```

### 3.2 使用示例

```bash
# 1. 快速连接本地 TiDB
tiup sql

# 2. 连接远程 MySQL
tiup sql mysql://admin:secret@10.0.1.50:3306/production

# 3. 执行单条 SQL
tiup sql -e "SHOW DATABASES" mysql://root@127.0.0.1:4000/

# 4. 执行 SQL 文件
tiup sql -f init_schema.sql -f seed_data.sql mysql://root@127.0.0.1:4000/app_db

# 5. 导出为 CSV
tiup sql --format csv -e "SELECT * FROM orders" mysql://root@127.0.0.1:4000/shop > orders.csv

# 6. 使用保存的连接
tiup sql -c prod

# 7. 管道执行
cat query.sql | tiup sql mysql://root@127.0.0.1:4000/
```

---

## 4. 项目结构与代码组织

### 4.1 目录结构

```
components/sql/
├── main.go                  # 入口：cobra 命令注册和根命令
├── cmd/
│   ├── root.go              # 根命令定义
│   ├── connect.go           # 连接相关子命令和选项
│   └── connection_mgmt.go   # 连接配置管理子命令
├── connect/
│   ├── dsn.go               # DSN 解析与构造
│   ├── pool.go              # 连接池管理
│   ├── tls.go               # TLS 配置与验证
│   ├── playground.go        # Playground 实例发现
│   └── config.go            # 连接配置文件读写
├── repl/
│   ├── repl.go              # REPL 主循环
│   ├── completer.go         # 自动补全
│   ├── highlighter.go       # SQL 语法高亮
│   ├── meta.go              # 元命令解析与执行
│   └── history.go           # 历史记录管理
├── batch/
│   ├── executor.go          # 批量执行器
│   ├── parser.go            # SQL 语句分割与解析
│   └── reader.go            # 文件/管道输入读取
├── format/
│   ├── formatter.go         # 格式化器接口
│   ├── table.go             # 表格格式
│   ├── csv.go               # CSV 格式
│   ├── json.go              # JSON 格式
│   ├── tsv.go               # TSV 格式
│   └── vertical.go          # 垂直表格格式
├── log/
│   ├── logger.go            # 查询日志记录
│   └── slow.go              # 慢查询检测
├── endpoint.go              # 端点类型定义
└── version.go               # 版本信息
```

### 4.2 核心类型定义

```go
// connect/dsn.go
package connect

type DSNConfig struct {
    Host         string
    Port         int
    User         string
    Password     string
    Database     string
    Protocol     string // "tcp" or "unix"
    Socket       string
    TLSConfig    *TLSConfig
    Timeout      time.Duration
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    Charset      string
    Collation    string
    Params       map[string]string
}

func ParseDSN(input string) (*DSNConfig, error) { /* ... */ }
func (c *DSNConfig) ToMySQLDSN() string { /* ... */ }

// connect/config.go
type ConnectionEntry struct {
    Name     string     `yaml:"name"`
    Host     string     `yaml:"host"`
    Port     int        `yaml:"port"`
    User     string     `yaml:"user"`
    Database string     `yaml:"database"`
    TLS      *TLSConfig `yaml:"tls,omitempty"`
}

type ConfigFile struct {
    Connections []ConnectionEntry `yaml:"connections"`
}
```

### 4.3 格式化器接口

```go
// format/formatter.go
type Formatter interface {
    // FormatHeaders 输出列标题
    FormatHeaders(w io.Writer, columns []string) error
    // FormatRow 输出一行数据
    FormatRow(w io.Writer, row []interface{}) error
    // FormatFooter 输出结果尾部信息（行数、耗时）
    FormatFooter(w io.Writer, rowCount int, duration time.Duration) error
    // Flush 刷新输出缓冲
    Flush() error
}
```

---

## 5. 构建与集成

### 5.1 Makefile 集成

在根 `Makefile` 中添加 `sql` 构建目标：

```makefile
.PHONY: sql
sql:
	@# Target: build tiup-sql component
	$(GOBUILD) -ldflags '$(LDFLAGS)' -o bin/tiup-sql ./components/sql

.PHONY: components
components: playground client cluster dm sql server
	@# Target: build all tiup components
```

### 5.2 依赖管理

新增依赖（已有部分在 `go.mod` 中）：

| 依赖 | 用途 | 是否新增 |
|------|------|---------|
| `github.com/xo/usql` | SQL REPL 引擎 | 已存在 |
| `github.com/go-sql-driver/mysql` | MySQL/TiDB 驱动 | 已存在 |
| `github.com/spf13/cobra` | CLI 框架 | 已存在 |
| `golang.org/x/term` | 密码密文输入 | 已存在 |
| `gopkg.in/yaml.v3` | 连接配置文件解析 | 已存在 |
| `github.com/olekukonko/tablewriter` | 表格输出格式化 | 需确认 |

### 5.3 测试策略

```
components/sql/
├── connect/
│   ├── dsn_test.go          # DSN 解析测试
│   ├── pool_test.go         # 连接池测试
│   └── config_test.go       # 配置文件读写测试
├── repl/
│   ├── completer_test.go    # 补全逻辑测试
│   ├── meta_test.go         # 元命令解析测试
│   └── repl_test.go         # REPL 集成测试
├── batch/
│   ├── executor_test.go     # 批量执行测试
│   └── parser_test.go       # SQL 分割测试
└── format/
    ├── table_test.go        # 表格格式测试
    ├── csv_test.go          # CSV 格式测试
    └── json_test.go         # JSON 格式测试
```

测试要点：
- **单元测试**：DSN 解析的各种输入格式、SQL 语句分割（含边界情况如字符串中的分号）、输出格式化的各种数据类型
- **集成测试**：需要真实 MySQL/TiDB 连接，通过 Docker 启动测试实例
- **安全测试**：密码不以明文出现在日志中、TLS 证书验证、SQL 注入防护

---

## 6. 安全考量

### 6.1 密码安全

- **命令行参数**：不支持 `--password=xxx` 直接传值（避免 `ps` 泄露），`-p` 无参数时交互式输入，`-p xxx` 仅支持从环境变量读取
- **连接配置文件**：密码字段可选使用加密存储，运行时通过 OS 密钥链或 `TIUP_SQL_PASSWORD_<NAME>` 环境变量提供
- **日志安全**：查询日志中不记录密码（连接 DSN 中密码部分以 `***` 替换）

### 6.2 TLS 证书验证

- 默认启用证书验证（`InsecureSkipVerify: false`）
- `--tls-skip-verify` 必须显式指定，并在连接时输出警告
- 证书路径不存在时给出明确错误提示

### 6.3 SQL 注入防护

- 所有元数据查询（数据库名、表名、列名）使用参数化查询
- 自动补全的候选列表从 `INFORMATION_SCHEMA` 获取，不经由用户输入拼接

### 6.4 输入安全

- 文件执行限制：仅读取 `.sql` 文件（可通过 `--file` 显式指定其他扩展名）
- 不支持 `SYSTEM` 命令或 `LOAD DATA LOCAL INFILE` 的自动执行（需通过元命令 `\system` 显式调用）

---

## 7. 里程碑规划

| 阶段 | 内容 | 预估 |
|------|------|------|
| **M1: 核心连接** | DSN 解析、基础连接、REPL、表格输出、Playground 发现 | 3 天 |
| **M2: 格式与批量** | CSV/JSON/TSV/垂直格式、文件执行、管道执行、错误处理 | 2 天 |
| **M3: 增强功能** | 自动补全、连接配置管理、TLS 支持、元命令 | 2 天 |
| **M4: 测试与文档** | 单元测试、集成测试、安全测试、用户文档 | 2 天 |

---

## 8. 与现有 `tiup client` 的兼容性

### 8.1 迁移策略

- `tiup sql` 作为 `tiup client` 的增强替代，初期两者并存
- `tiup sql --playground` 行为与 `tiup client` 完全一致
- 后续版本中 `tiup client` 标记为 deprecated，引导用户使用 `tiup sql`

### 8.2 行为差异

| 特性 | `tiup client` | `tiup sql` |
|------|---------------|------------|
| 连接目标 | 仅本地 playground | 任意 MySQL/TiDB |
| DSN 支持 | 无 | 完整 DSN 支持 |
| 输出格式 | 仅表格 | 表格/CSV/JSON/TSV/垂直 |
| 文件执行 | 不支持 | 支持 |
| 自动补全 | 无 | SQL 关键字 + 元数据 |
| 连接配置 | 无 | 持久化连接管理 |
| TLS | 不支持 | 完整 TLS 支持 |
