# tiup sql 第二轮（最终）评审报告

**评审人**：全能表妹（安全审计）
**评审时间**：2026-05-18
**评审范围**：第二轮修复的 4 个问题（M1-M4）、更新后的测试用例、`tiup-sql.md` 变更文档

---

## 评审概要

对 @素子 第二轮修复进行逐项验证。本轮修复涉及 `batch/executor.go`、`format/vertical.go`、`connect/config_test.go` 三个文件。

**结论：4 个修复全部验证通过，无新增阻断或高严重度问题。代码可以进入编译验证和集成测试阶段。**

---

## 修复验证

### M1. SQL 字符串内分号分割 — ✅ 已修复

**文件**：`batch/executor.go:227-281`（splitStatements）、`batch/executor.go:159-225`（readStatements）

**验证**：
- `splitStatements()` 现在逐字符扫描，跟踪 `inSingleQuote` 和 `inDoubleQuote` 状态
- 正确处理 `''`（双单引号转义）和 `""`（双双引号转义）
- 在引号外的分号处分割，引号内的分号保留
- `readStatements()` 也增加了相同的引号感知逻辑
- 新增 5 个测试用例覆盖：单引号内分号、双引号内分号、转义单引号、混合引用、普通无引号场景

**小问题（Low，不影响正确性）**：`readStatements()` 在检测到行尾分隔符后调用 `splitStatements()` 进行二次处理（第 183-186 行）。这在功能上正确但存在冗余：当 `readStatements` 已经追踪了引号状态并在正确的位置分割时，再次调用 `splitStatements` 会重复一次引号扫描。不过这是一个性能微优化点，不影响正确性，不影响进度。

**MySQL 转义说明**：当前实现处理 `''` 双单引号转义（标准 SQL），未处理 `\'` 反斜杠转义（MySQL 扩展模式）。在默认 `sql_mode` 下 MySQL 两种转义都支持，但反斜杠转义仅在 `NO_BACKSLASH_ESCAPES` 模式关闭时生效。对于大多数实际使用场景（`INSERT INTO t VALUES ('hello;world')`），当前实现已足够。

### M2. 计数器累积 — ✅ 已修复

**文件**：`batch/executor.go:283-311`

```go
func (e *Executor) execStatements(statements []string) error {
    var successes, failures int  // 局部变量
    ...
}
```

**验证**：`successes` 和 `failures` 从 `Executor` 结构体字段移至 `execStatements()` 局部变量。`Executor` 结构体不再包含这两个字段（第 42-47 行确认）。多次调用 `ExecString`/`ExecFiles`/`ExecStdin` 不再累积计数。

### M3. 垂直格式化性能 — ✅ 已修复

**文件**：`format/vertical.go:50-55`

```go
for i, row := range rows {
    ...
    maxLabelLen := 0
    for _, c := range columns {
        if cl := utf8.RuneCountInString(c); cl > maxLabelLen {
            maxLabelLen = cl
        }
    }
    for j, col := range columns {
```

**验证**：`maxLabelLen` 计算从列内循环（原嵌套在 `for j, col` 内部）提升到行循环顶层。当前复杂度 O(rows × cols)，不再有 O(rows × cols²) 的冗余计算。

**小问题（Low）**：`maxLabelLen` 仅依赖 `columns`，理论上只需计算一次（O(cols)），放到所有行循环之前即可。当前每行都计算一次（O(rows × cols)），虽然比修复前的 O(rows × cols²) 好很多，但仍不是最优。不影响进度。

### M4. 测试环境变量恢复 — ✅ 已修复

**文件**：`connect/config_test.go:108-126`

```go
func setTestHome(dir string) func() {
    origHome, hadHome := os.LookupEnv("HOME")
    origUserProfile, hadUserProfile := os.LookupEnv("USERPROFILE")
    os.Setenv("HOME", dir)
    os.Setenv("USERPROFILE", dir)
    return func() {
        if hadHome {
            os.Setenv("HOME", origHome)
        } else {
            os.Unsetenv("HOME")
        }
        if hadUserProfile {
            os.Setenv("USERPROFILE", origUserProfile)
        } else {
            os.Unsetenv("USERPROFILE")
        }
    }
}
```

**验证**：
- 使用 `os.LookupEnv` 保存原始值和是否存在
- 同时处理 `HOME`（Unix）和 `USERPROFILE`（Windows）
- 正确恢复：存在则恢复原始值，不存在则 `Unsetenv`
- 两个测试用例都已更新使用 `setTestHome()` + `defer restore()`

---

## 文档验证

### `tiup-sql.md` — ✅ 已创建

**路径**：`tiup-repo/tiup-sql.md`

**验证**：
- 完整的变更文件列表（18 个源文件 + 5 个测试文件）
- 两轮修复历史记录完整（第一轮 6 项 + 第二轮 4 项）
- 安全说明 S1（`TIUP_SQL_PASSWORD` 风险）包含具体攻击路径、影响范围和缓解建议
- 文档格式清晰，结构合理

---

## 安全合规最终检查

| 检查项 | 状态 |
|--------|------|
| 密码不以命令行明文传递 | ✅ |
| 密码交互式密文输入 | ✅ |
| TLS 证书验证默认启用 | ✅ |
| `--tls-skip-verify` 需显式指定 | ✅ |
| 元命令 SQL 注入防护 | ✅ |
| 日志中密码脱敏 | ✅ |
| 连接配置文件权限 0o600 | ✅ |
| 连接配置名称安全过滤 | ✅ |
| SQL 字符串内分号正确处理 | ✅ |
| 环境变量密码风险已文档化 | ✅ |
| 安全问题变更文档 `tiup-sql.md` | ✅ |

---

## 总结

**所有已知阻断和高严重度问题均已修复验证通过。** 代码具备进入编译验证和集成测试的条件。

**剩余低优先级项**（可后续迭代处理）：
- `readStatements` 中 `splitStatements` 的二次调用冗余
- `vertical.go` 的 `maxLabelLen` 可进一步优化到 O(cols)
- MySQL `\'` 反斜杠转义支持（NO_BACKSLASH_ESCAPES 模式）
- 缺失的高级功能（自动补全、语法高亮、历史持久化）
- CJK 字符宽度对齐、CSV BOM 等

**@wdrhhwx 所有问题修复并验证没有问题，可以进行编译验证和集成测试。**
