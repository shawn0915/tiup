# tiup sql 第一轮代码审阅报告

**审阅人**：元芳（高级测试工程师）
**审阅时间**：2026-05-17
**审阅范围**：`components/sql/` 全部源码及测试文件
**设计文档**：`output/tiup-sql-design.md`

---

## 审阅概要

对 `components/sql/` 下全部 20 个文件（15 个源文件 + 5 个测试文件）进行了逐文件审阅。整体架构与设计文档基本一致，模块划分清晰。但发现 **2 个编译阻断问题**、**4 个高严重度缺陷**（含安全漏洞）、**6 个中等问题** 和若干低优先级代码质量问题。

---

## 1. 阻断级（Critical — 无法编译/功能完全不可用）

### C1. `dsn.go` 缺少必要 import，编译失败

**文件**：`connect/dsn.go:302-341`

`tlsxConfigName()` 函数使用了 `crypto/tls`、`crypto/x509`、`os` 包，但文件头部的 import 中没有这三个包：

```go
// 当前 import（第16-22行）
import (
    "fmt"
    "net/url"
    "strconv"
    "strings"
    "time"
)
// 缺少: "crypto/tls", "crypto/x509", "os"
```

`tlsxConfigName()` 中引用了 `tls.Config{}`、`x509.NewCertPool()`、`os.ReadFile()` 等，**项目无法编译通过**。

**修复**：在 `dsn.go` import 中添加 `"crypto/tls"`、`"crypto/x509"`、`"os"`。或者更好的做法是将 `tlsxConfigName()` 中的 TLS 构建逻辑移入 `connect/tls.go`，复用已有的 `BuildTLSConfig()`，避免重复代码。

---

### C2. `RegisterTLSConfig` 是空实现，TLS 连接完全不工作

**文件**：`connect/tls.go:26-28`

```go
func RegisterTLSConfig(name string, config *tls.Config) error {
    return nil  // 空实现！实际没有注册到 MySQL 驱动
}
```

`tlsxConfigName()`（dsn.go:335）调用此函数注册 TLS 配置，但该函数什么也没做。TLS 连接名称注册后，MySQL 驱动找不到对应的 TLS 配置，所有 TLS 连接都会失败。

`tls.go` 中已经有正确的 `BuildTLSConfig()` 函数，但它从未被任何地方调用。

**修复**：
- `RegisterTLSConfig` 应调用 `mysql.RegisterTLSConfig(name, config)`（`github.com/go-sql-driver/mysql` 提供）
- 或者让 `tlsxConfigName()` 直接使用 `BuildTLSConfig()` 构建配置后注册

---

## 2. 高严重度（High — 安全/正确性问题）

### H1. `\d` 和 `\u` 元命令存在 SQL 注入漏洞

**文件**：`repl/repl.go:229-237`

```go
case "\\d":
    if arg != "" {
        return r.execQueryWrapped(fmt.Sprintf("DESCRIBE %s", arg))  // 未转义！
    }
case "\\u", "\\use":
    if arg == "" {
        return fmt.Errorf("usage: \\u <database>")
    }
    return r.execQueryWrapped(fmt.Sprintf("USE %s", arg))  // 未转义！
```

用户输入直接拼接进 SQL 语句。设计文档 6.3 节明确要求"所有元数据查询使用参数化查询"。虽然 MySQL 不支持标识符参数化，但至少应使用反引号包裹并转义内部反引号。

**修复**：添加标识符转义函数：
```go
func quoteIdentifier(name string) string {
    escaped := strings.ReplaceAll(name, "`", "``")
    return "`" + escaped + "`"
}
```

### H2. `--password` flag 允许直接传值，与设计要求冲突

**文件**：`cmd/root.go:111`

```go
cmd.Flags().StringVarP(&flags.password, "password", "p", "",
    "Database password (use without value for interactive prompt)")
```

设计文档 6.1 节明确规定："不支持 `--password=xxx` 直接传值（避免 `ps` 泄露）"。当前实现允许 `--password mysecret`，密码会出现在进程列表中。

**修复**：将 `--password` 改为不需要值的 bool flag，或者在检测到有值时输出警告并仍进入交互式输入。

### H3. `ToMySQLDSN()` 未对密码进行 URL 编码

**文件**：`connect/dsn.go:240-243`

```go
if c.Password != "" {
    buf.WriteByte(':')
    buf.WriteString(c.Password)  // 未编码特殊字符
}
```

如果密码包含 `@`、`:`、`/`、`%`、`#` 等特殊字符，生成的 DSN 会被 MySQL 驱动错误解析。例如密码 `p@ss` 会生成 `root:p@ss@tcp(...)` 导致连接失败。

**修复**：使用 `url.QueryEscape(c.Password)` 或 `net/url.PathEscape()` 对密码进行编码。

### H4. 密码输入退格键清除整个输入而非最后一个字符

**文件**：`cmd/root.go:354-357`

```go
if b[0] == 127 || b[0] == 8 { // Backspace
    if buf.Len() > 0 {
        buf.Reset()  // BUG: 清除了全部内容，应只删除最后一个字符
    }
    continue
}
```

**修复**：改为 `buf.Truncate(buf.Len() - 1)`（注意需先检查 `buf.Len() > 0`）。

---

## 3. 中等严重度（Medium — 功能缺陷/逻辑问题）

### M1. `MaxRows` 选项声明但未实现

**文件**：`batch/executor.go:38`

`ExecutorOptions.MaxRows` 已声明且在 `root.go:277` 中传入 0，但 `execOne()` 中没有任何地方检查或限制返回的行数。设计文档要求支持 `--max-rows` 参数。

**修复**：在 `execOne()` 读取行后增加行数检查：
```go
if e.opts.MaxRows > 0 && len(resultRows) >= e.opts.MaxRows {
    break
}
```

### M2. 成功/失败计数器在多次 exec 调用间累积

**文件**：`batch/executor.go:47-48`

`successes` 和 `failures` 作为 `Executor` 的字段，在 `ExecString`、`ExecFiles`、`ExecStdin` 之间累积。如果同一 Executor 被多次调用，结束消息显示的是累积值而非单次执行值。

**修复**：将计数器移入 `execStatements()` 局部变量。

### M3. 垂直格式化器在内层循环中重复计算 `maxLabelLen`

**文件**：`format/vertical.go:54-58`

```go
for i, row := range rows {
    for j, col := range columns {
        val := ""
        if j < len(row) {
            val = formatValue(row[j])
        }
        maxLabelLen := 0           // 每次内层循环都重新计算
        for _, c := range columns {
            if utf8.RuneCountInString(c) > maxLabelLen {
                maxLabelLen = utf8.RuneCountInString(c)
            }
        }
```

`maxLabelLen` 仅依赖 `columns`，在每行每列的循环体内 O(N) 扫描全部列名。总共 O(rows * cols²) 的复杂度。

**修复**：将 `maxLabelLen` 计算移到外层循环之前。

### M4. 慢查询阈值解析失败时静默回退

**文件**：`repl/repl.go:63-65`

```go
if opts.SlowThreshold != "" {
    d, err := time.ParseDuration(opts.SlowThreshold)
    if err != nil {
        d = time.Second  // 静默回退，用户无感知
    }
```

用户提供了无效的 `--slow-threshold` 值（如 `"abc"`），系统静默回退到 1 秒。

**修复**：应返回错误或至少输出警告到 stderr。

### M5. TLS 配置注册名冲突且从不清理

**文件**：`connect/dsn.go:334`

TLS 配置以 `tiup-sql-{host}-{port}` 为名称注册到 MySQL 驱动。如果同一进程多次连接同一 host:port（例如连接管理场景），注册会冲突。已注册的配置也从不会清理。

### M6. 连接配置缓存无过期/失效机制

**文件**：`connect/config.go:29-32`

```go
var (
    configMu    sync.Mutex
    configCache *ConfigFile
)
```

`configCache` 一旦加载就永不失效。如果在运行期间外部修改了 `connections.yaml`，后续操作读到的是过期数据。多实例场景下也可能读到过时缓存。

---

## 4. 低严重度（Low — 代码质量/完整性）

### L1. 未使用的变量和类型

| 文件 | 内容 | 说明 |
|------|------|------|
| `version.go:19` | `sqlVersion` | 声明后未使用 |
| `endpoint.go:17-22` | `Endpoint` struct | 定义后未在任何代码中引用 |

### L2. 设计文档与实现的差距

设计文档指定了以下文件/功能，当前实现中缺失：

| 设计文件 | 状态 | 说明 |
|----------|------|------|
| `repl/completer.go` | 缺失 | 自动补全功能未实现 |
| `repl/highlighter.go` | 缺失 | 语法高亮未实现 |
| `repl/history.go` | 缺失 | 历史记录持久化未实现 |
| `repl/meta.go` | 缺失 | 元命令逻辑内联在 repl.go |
| `batch/parser.go` | 缺失 | SQL 解析逻辑内联在 executor.go |
| `batch/reader.go` | 缺失 | 文件读取逻辑内联在 executor.go |
| `log/slow.go` | 缺失 | 慢查询检测内联在 repl.go |

缺失的元命令：`\w`（写文件）、`\pager`（分页器）、`\!`（系统命令）、`\warnings`、`\e`（编辑器）

缺失的环境变量支持：`TIUP_SQL_PASSWORD`

### L3. DSN 解析在 `user:pass@host:port` 格式下不支持密码中的 `@`

**文件**：`connect/dsn.go:159`

```go
atIdx := strings.Index(input, "@")  // 取第一个 @
```

如果密码包含 `@`（如 `user:p@ss@host:4000`），解析会错误分割。应从末尾查找最后一个 `@`：
```go
atIdx := strings.LastIndex(input, "@")
```

### L4. 连接配置未存储密码

`ConnectionEntry` 结构体没有 `Password` 字段，`SaveConnection` 也不存储密码。设计文档要求支持密码持久化（通过 bcrypt 或 OS 密钥链）。当前每次使用保存的连接都需要重新输入密码。

### L5. 测试覆盖不足

- `repl/` 包没有任何测试文件
- `batch/` 没有端到端执行测试（只测试了语句分割）
- `connect/tls.go` 没有测试
- `connect/pool.go` 没有测试
- `cmd/` 没有测试

建议在 M4 阶段补充。

### L6. 表格格式化 CJK 字符对齐问题

`table.go` 使用 `utf8.RuneCountInString` 计算列宽，但 `%-*s` 格式化按 rune 数填充空格。CJK 字符占 2 显示列宽但只算 1 rune，导致 CJK 内容的列宽计算错误、对齐偏移。

### L7. CSV 格式缺少设计文档要求的 BOM 头

设计文档 2.5.2 节要求 CSV 输出带 UTF-8 BOM（Excel 兼容），当前 `csv.go` 未写入 BOM。

---

## 5. 已确认的正常实现

以下方面实现良好，符合设计要求：

- DSN 多格式解析（URL、user@host、host:port）整体逻辑正确
- `SanitizedDSN()` 密码脱敏机制正确
- 批量执行器的 `stop/continue/abort` 错误策略框架正确
- 查询日志的 DSN 脱敏和错误截断合理
- 连接配置文件的 CRUD 操作正确，文件权限 0600 安全
- `Playground` 发现复用了 tiup 现有路径逻辑
- 跨平台密码输入的 build tag 分离正确
- REPL 的 SIGINT/SIGTERM 信号处理正确
- 批量执行器支持目录递归扫描 `.sql` 文件

---

## 6. 优先级排序和建议

| 优先级 | 编号 | 描述 | 影响 |
|--------|------|------|------|
| **P0-立即修复** | C1, C2 | 编译失败 + TLS 不工作 | 功能完全不可用 |
| **P1-本迭代修复** | H1, H4 | SQL 注入 + 退格 bug | 安全/基本功能 |
| **P1-本迭代修复** | H2, H3 | 密码传值 + URL 编码 | 安全/兼容性 |
| **P2-下迭代修复** | M1-M6 | 功能缺陷和逻辑问题 | 完整性/健壮性 |
| **P3-后续版本** | L1-L7 | 代码质量和设计差距 | 工程质量 |

**建议修复顺序**：C1 → C2 → H1 → H4 → H3 → H2 → M1 → M3 → M2 → M4 → 其余

---

## 7. 验证建议

修复后需要验证：
1. `go build ./components/sql/` 编译通过
2. 带 TLS 参数的连接能正常建立
3. `\d users\` DROP TABLE users; --` 等注入尝试被正确处理
4. 密码输入时退格只删除一个字符
5. 密码含特殊字符（`@`、`:`、`%`）时连接正常
6. `--max-rows` 正确限制返回行数
