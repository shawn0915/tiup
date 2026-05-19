# tiup sql 综合评审报告

**评审人**：全能表妹（安全审计）
**评审时间**：2026-05-18
**评审范围**：`components/sql/` 全部源码及测试文件（15 源文件 + 5 测试文件）
**设计文档**：`output/tiup-sql-design.md`
**第一轮审阅**：元芳（已修复 6 个阻断/高严重度问题）

---

## 评审概要

本次评审在元芳第一轮审阅（发现 2 阻断 + 4 高 + 6 中 + 7 低，共 19 项问题）的基础上进行综合安全审计和质量验证。

**第一轮修复验证**：
- C1（dsn.go import 缺失）— ✅ 已修复，TLS 逻辑已分离至 `tls.go`
- C2（RegisterTLSConfig 空实现）— ✅ 已修复，现正确调用 `mysql.RegisterTLSConfig()`
- H1（SQL 注入）— ✅ 已修复，`isValidIdentifier()` + `escapeIdentifier()` 已添加
- H2（密码 flag 传值）— ✅ 已修复，`--password` 改为 bool 标志
- H3（密码 URL 编码）— ✅ 已修复，`ToMySQLDSN()` 使用 `url.QueryEscape()`
- H4（退格键 bug）— ✅ 已修复，改用 `runes` 切片逐字符删除

**本轮新发现**：0 阻断 / 1 高 / 4 中 / 5 低，共 10 项

---

## 1. 高严重度（High — 安全问题）

### S1. 密码以明文存储在内存和环境变量中，进程 dump 可泄露

**文件**：`cmd/root.go:199-208`

```go
if envPW := os.Getenv("TIUP_SQL_PASSWORD"); envPW != "" {
    dsnConfig.Password = envPW
}
```

**攻击路径**：
1. 用户设置环境变量 `TIUP_SQL_PASSWORD=secret`
2. 攻击者通过 `/proc/<pid>/environ`（Linux）或进程诊断工具读取进程环境变量
3. 密码明文泄露

同时，`DSNConfig.Password` 字段在内存中以明文 `string` 存储，Go `string` 不可变且无法主动清零，Go 垃圾回收后内存页可能被操作系统交换到磁盘（swap），密码残留可被取证工具提取。

**影响**：中等 — 环境变量传密码是常见做法，但存在已知风险

**修复建议**：
- 短期：在帮助文本中警告 `TIUP_SQL_PASSWORD` 环境变量的安全风险，推荐使用连接配置 + 交互输入
- 长期：考虑使用 `runtime.MemStats` 控制或 `[]byte` 缓冲区 + `crypto/subtle.ConstantTimeCompare` 减少内存暴露时间

**严重度定级理由**：虽然该模式与 `mysql` 命令行客户端等成熟工具一致，但设计文档 6.1 节明确要求凭据安全管理，此问题应被记录和跟踪。

---

## 2. 中等严重度（Medium — 功能/逻辑/健壮性）

### M1. `splitStatements()` 在 SQL 字符串内的分号处错误分割

**文件**：`batch/executor.go:200-210`、`batch/executor.go:161-198`

```go
func (e *Executor) splitStatements(input string) []string {
    parts := strings.Split(input, e.opts.Delimiter)  // 无引号感知
    ...
}
```

`splitStatements()` 和 `readStatements()` 都不处理 SQL 字符串字面量内的分号：

```sql
INSERT INTO t VALUES ('hello;world');
```

会被错误分割为：
1. `INSERT INTO t VALUES ('hello`
2. `world')`

测试文件 `executor_test.go:111-131` 已明确记录此限制（"simple parser splits on all semicolons including those in strings"），但这是功能性缺陷，影响带分号的字符串数据的批量导入。

**影响**：带分号/注释分隔符的数据批量导入会失败或产生错误 SQL

**修复建议**：实现基本的引号感知分割：
- 跟踪当前是否在 `'...'`（单引号）或 `"..."`（双引号）或 `/*...*/`（块注释）或 `--...`（行注释）内
- 仅在非引号/非注释位置的分号处分割
- 注意 MySQL 字符串转义：`\'`、`\\` 不结束字符串

### M2. TLS 配置名称使用 host:port 拼接，host 可能含特殊字符

**文件**：`connect/dsn.go:304-313`、`connect/tls.go:79`

```go
// dsn.go
return fmt.Sprintf("tiup-sql-%s-%d", c.Host, c.Port)

// tls.go
name := fmt.Sprintf("tiup-sql-%s-%d", c.Host, c.Port)
```

如果 `c.Host` 包含点号、连字符以外的特殊字符（理论上不太可能但不排除），可能导致 `mysql.RegisterTLSConfig` 的内部 map 冲突或不可预期的行为。虽然 DSN 解析阶段会验证 host 格式，但 `parseUserHost()` 和 `parseHostPort()` 并未强制 host 只允许合法字符。

**影响**：低概率 TLS 注册冲突

**修复建议**：对 host 使用安全的哈希或仅保留 `[a-zA-Z0-9.-]` 字符。

### M3. `applyFlagOverrides()` 逻辑与设计意图矛盾

**文件**：`cmd/root.go:243-262`

```go
func applyFlagOverrides(cfg *connect.DSNConfig) {
    if flags.host != "127.0.0.1" || cfg.Host == "" {
        cfg.Host = flags.host
    }
    if flags.port != 4000 || cfg.Port == 0 {
        cfg.Port = flags.port
    }
    ...
}
```

条件 `flags.host != "127.0.0.1" || cfg.Host == ""` 的意图是：如果用户显式指定了非默认 host，或者 DSN 中 host 为空，则覆盖。但问题是：

- 当 DSN 指定了 `host=10.0.0.1`，且用户也通过 `--host 10.0.0.1`（与 DSN 相同的值）时，不会覆盖 — 这是正确的
- 但当用户 DSN 指定了 `host=10.0.0.1`，用户没有传 `--host`（默认 127.0.0.1）时，由于 `flags.host == "127.0.0.1"` 且 `cfg.Host != ""`，不会覆盖 — 这也是正确的

然而，对于 `port` 的逻辑 `flags.port != 4000 || cfg.Port == 0`：
- DSN 指定了 `port=3306`，用户没传 `--port`（默认 4000），由于 `flags.port == 4000` 且 `cfg.Port != 0`，不覆盖 — 正确
- DSN 指定了 `port=4000`，用户传 `--port 4000`，不覆盖 — 正确

总体逻辑看似正确，但代码意图不够清晰，容易在后续维护中引入 bug。

**影响**：代码可维护性风险

**修复建议**：改为显式跟踪用户是否传入了 flag（使用 `cmd.Flags().Changed("host")` 等），逻辑更清晰。

### M4. `config_test.go` 环境变量恢复存在 bug

**文件**：`connect/config_test.go:93-95`

```go
func TestConfigFile_FileCreation(t *testing.T) {
    tmpDir := t.TempDir()
    os.Setenv("HOME", tmpDir)
    defer os.Setenv("HOME", os.Getenv("HOME"))  // BUG: 此时 HOME 已经是 tmpDir
    ...
}
```

在 `os.Setenv("HOME", tmpDir)` 之后，`os.Getenv("HOME")` 返回 `tmpDir`，所以 `defer` 恢复的值仍然是 `tmpDir`，测试结束后 `HOME` 环境变量不会被恢复为原始值。对比同文件 `TestConfigFile_SaveAndLoad`（第 24-26 行）的正确写法：

```go
home := os.Getenv("HOME")
os.Setenv("HOME", tmpDir)
defer os.Setenv("HOME", home)  // 正确：保存原始值
```

**影响**：测试污染，可能影响后续测试（虽然 Go 通常并行运行测试时每个包独立）

**修复建议**：使用与 `TestConfigFile_SaveAndLoad` 相同的模式。

---

## 3. 低严重度（Low — 代码质量/完整性）

### L1. 全局可变 `flags` 变量导致测试困难

**文件**：`cmd/root.go:72`

```go
var flags globalFlags
```

`flags` 是包级全局变量，所有 flag 绑定直接写入此变量。这使得单元测试无法隔离不同 flag 组合的场景。虽然当前 `cmd/` 包没有测试文件（L5），但如果未来添加测试，全局可变状态会成为阻碍。

**修复建议**：将 `flags` 嵌入 `cobra.Command` 的 `Context` 或使用依赖注入。

### L2. `execQuery()` 使用 `Query()` 而非 `Exec()` 处理非 SELECT 语句

**文件**：`repl/repl.go:163-165`

```go
func (r *REPL) execQuery(query string) {
    rows, err := r.db.Query(query)
```

所有 SQL 语句（包括 `INSERT`、`UPDATE`、`DELETE`、`CREATE`、`DROP` 等）都通过 `Query()` 执行。对于 DML/DDL 语句，应使用 `Exec()` 以：
- 避免在服务器端创建不必要的结果集
- 获取 `RowsAffected()` 等受影响行数信息
- 减少客户端与服务器之间的网络开销

当前实现虽然功能上可以工作（MySQL 驱动对 DML 的 `Query()` 返回空结果集），但效率不高。

**修复建议**：添加简单的语句类型判断（如前缀匹配 `SELECT`、`SHOW`、`DESCRIBE`、`EXPLAIN` 用 `Query()`，其他用 `Exec()`）。

### L3. `execFile()` 在 `\i` 元命令中不处理目录路径

**文件**：`repl/repl.go:340-377`

`\i <file>` 只能执行单个文件，不支持目录。如果用户传入目录路径，`os.Open()` 会失败。设计文档提到 `\i <file>` 执行 SQL 文件，但对比批量执行器的 `execDirectory()` 功能，REPL 内的 `\i` 功能较弱。

**影响**：功能不一致，但非安全风险

### L4. `formatValue()` 对 `[]byte` 使用 `string()` 转换可能产生非 UTF-8 输出

**文件**：`format/table.go:133-134`

```go
case []byte:
    return string(val)
```

MySQL 的 `BLOB`、`BINARY` 列返回 `[]byte`，如果内容是二进制数据（如图像、加密数据），`string()` 转换可能产生乱码或破坏终端显示。

**修复建议**：检测是否为有效 UTF-8，非 UTF-8 内容显示为 `<BINARY n bytes>` 占位符。

### L5. 测试覆盖仍不足（延续第一轮 L5）

当前测试仅覆盖纯逻辑模块（DSN 解析、配置 CRUD、格式化输出、语句分割、日志记录），缺少：

- `cmd/root.go` 的 flag 解析和执行路径测试
- `repl/repl.go` 的 REPL 循环和元命令测试
- `batch/executor.go` 的 `execOne()` 和错误策略测试
- `connect/tls.go` 的 TLS 配置注册测试
- `connect/pool.go` 的数据库连接测试（需 mock）

**建议**：在后续迭代中优先补充 `cmd/`、`repl/` 和 `connect/tls.go` 的测试。

---

## 4. 第一轮遗留问题状态

| 编号 | 描述 | 状态 | 说明 |
|------|------|------|------|
| M1（第一轮） | MaxRows 未实现 | 🔴 未修复 | 代码中仍无 MaxRows 行数限制逻辑 |
| M2（第一轮） | 成功/失败计数器累积 | 🔴 未修复 | `successes`/`failures` 仍为 Executor 字段 |
| M3（第一轮） | 垂直格式化 maxLabelLen 重复计算 | 🔴 未修复 | 仍在内层循环中 O(N²) 计算 |
| M4（第一轮） | 慢查询阈值解析静默回退 | 🔴 未修复 | 仍静默回退到 1s |
| M5（第一轮） | TLS 注册名冲突 | 🟡 部分修复 | 已改用 `RegisterTLSConfig` 带锁，但同 host:port 重复注册仍会报错 |
| M6（第一轮） | 配置缓存无过期 | 🔴 未修复 | configCache 仍无失效机制 |
| L1-L7（第一轮） | 代码质量和设计差距 | 大部分 🔴 未修复 | 预期在后续迭代处理 |

---

## 5. 安全合规检查

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 密码不以命令行明文传递 | ✅ 通过 | `--password` 改为 bool flag |
| 密码交互式密文输入 | ✅ 通过 | 使用 term raw mode 读取 |
| TLS 证书验证默认启用 | ✅ 通过 | `InsecureSkipVerify: false` 默认 |
| `--tls-skip-verify` 需显式指定 | ✅ 通过 | Bool flag 默认 false |
| 元命令 SQL 注入防护 | ✅ 通过 | `isValidIdentifier()` + 反引号包裹 |
| 日志中密码脱敏 | ✅ 通过 | `SanitizedDSN()` + `sanitizeDSN()` |
| 连接配置文件权限 | ✅ 通过 | `0o600` 仅所有者读写 |
| 连接配置名称安全 | ✅ 通过 | `strings.ContainsAny(name, "/\\:*?\"<>|")` 拒绝非法字符 |
| 环境变量密码风险 | ⚠️ 已知风险 | 详见 S1，建议添加文档警告 |
| SQL 字符串内分号处理 | ⚠️ 功能缺陷 | 详见 M1，影响批量导入 |
| 输入密码内存生命周期 | ⚠️ 已知限制 | Go string 不可变，无法主动清零 |

---

## 6. 设计文档符合度

| 模块 | 符合度 | 差距 |
|------|--------|------|
| 连接管理 — DSN 解析 | 95% | 密码含 `@` 的 user@host 格式（L3 延续）未处理 |
| 连接管理 — TLS | 90% | 基本功能完整，重复注册需处理 |
| 连接管理 — Playground 发现 | 100% | 完全符合 |
| 连接管理 — 连接配置 | 85% | 缺少密码持久化和缓存失效 |
| 交互式 REPL | 70% | 自动补全、语法高亮、历史持久化未实现 |
| 批量执行 | 85% | MaxRows 未实现，字符串内分号分割有缺陷 |
| 输出格式化 | 95% | CJK 对齐、CSV BOM 未处理 |
| 日志与诊断 | 90% | 基本完整，慢查询阈值回退需提示 |

---

## 7. 总结与建议

### 整体评价

代码架构清晰，模块划分合理，与设计文档高度一致。第一轮发现的 6 个阻断/高严重度问题已全部正确修复，安全态势明显改善。当前无阻断级问题。

### 需要关注的问题

1. **必须修复**（影响数据正确性）：
   - **M1**：SQL 字符串内分号分割 — 影响批量数据导入

2. **建议修复**（影响健壮性）：
   - **M2**：计数器累积
   - **M3**：垂直格式化性能
   - **M4**：测试环境变量恢复 bug

3. **可延后**（后续迭代）：
   - 第一轮遗留的 M4（慢查询回退）、M5（TLS 冲突）、M6（缓存过期）
   - 缺失的高级功能（自动补全、语法高亮、历史持久化）

### 结论

当前代码状态适合进行编译验证和基本功能测试。建议在 M1（字符串分号分割）修复后进入 M4 测试阶段。安全层面无阻断问题，剩余 S1（环境变量密码风险）为行业共性问题，建议通过文档方式告知用户。

---

**评审完成。所有问题修复并验证没有问题后请通知。**
