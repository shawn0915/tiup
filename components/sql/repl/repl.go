// Copyright 2025 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pingcap/tiup/components/sql/connect"
	"github.com/pingcap/tiup/components/sql/format"
	"github.com/pingcap/tiup/components/sql/log"
)

// Options holds REPL configuration.
type Options struct {
	Format        string
	Timing        bool
	SlowThreshold string
	Pager         string
}

// REPL manages the interactive SQL session.
type REPL struct {
	db         connect.DB
	formatter  format.Formatter
	logger     *log.QueryLogger
	opts       Options
	prompt     string
	slowThresh time.Duration
}

// New creates a new REPL instance.
func New(db connect.DB, formatter format.Formatter, logger *log.QueryLogger, opts Options) (*REPL, error) {
	r := &REPL{
		db:        db,
		formatter: formatter,
		logger:    logger,
		opts:      opts,
		prompt:    "mysql> ",
	}

	if opts.SlowThreshold != "" {
		d, err := time.ParseDuration(opts.SlowThreshold)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid slow threshold %q, using default 1s\n", opts.SlowThreshold)
			d = time.Second
		}
		r.slowThresh = d
	}

	return r, nil
}

// Run starts the interactive read-eval-print loop.
func (r *REPL) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	fmt.Fprintf(os.Stdout, "Welcome to tiup sql.\n\n")

	reader := bufio.NewReader(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Fprint(os.Stdout, r.prompt)

		query, err := r.readQuery(reader)
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(os.Stdout)
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}

		if strings.HasPrefix(query, "\\") {
			if err := r.handleMetaCommand(query); err != nil {
				if err == io.EOF {
					return nil
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		r.execQuery(query)
	}
}

func (r *REPL) readQuery(reader *bufio.Reader) (string, error) {
	var buf strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && buf.Len() > 0 {
				return buf.String(), io.EOF
			}
			return buf.String(), err
		}

		line = strings.TrimRight(line, "\r\n")
		buf.WriteString(line)
		buf.WriteByte(' ')

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasSuffix(trimmed, ";") {
			break
		}

		if strings.HasPrefix(trimmed, "\\") {
			break
		}

		fmt.Fprint(os.Stdout, "    -> ")
	}

	return strings.TrimSpace(buf.String()), nil
}

func (r *REPL) execQuery(query string) {
	start := time.Now()

	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	isQuery := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "EXPLAIN")

	if isQuery {
		r.execSelect(query, start)
	} else {
		r.execDML(query, start)
	}
}

func (r *REPL) execSelect(query string, start time.Time) {
	rows, err := r.db.Query(query)
	elapsed := time.Since(start)

	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			fmt.Fprintf(os.Stderr, "ERROR %d (%s): %s\n", mysqlErr.Number, mysqlErr.SQLState, mysqlErr.Message)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		}
		r.logger.Log(query, 0, elapsed, err)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to get columns: %v\n", err)
		return
	}

	var resultRows [][]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: scan failed: %v\n", err)
			return
		}
		resultRows = append(resultRows, values)
	}

	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: rows iteration: %v\n", err)
		return
	}

	rowCount := len(resultRows)
	r.formatter.FormatResult(os.Stdout, columns, resultRows, rowCount, elapsed)

	if r.opts.Timing && r.slowThresh > 0 && elapsed > r.slowThresh {
		fmt.Fprintf(os.Stderr, " \u26a0 slow query (>%.2fs)\n", r.slowThresh.Seconds())
	}

	r.logger.Log(query, rowCount, elapsed, nil)
}

func (r *REPL) execDML(query string, start time.Time) {
	result, err := r.db.Exec(query)
	elapsed := time.Since(start)

	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			fmt.Fprintf(os.Stderr, "ERROR %d (%s): %s\n", mysqlErr.Number, mysqlErr.SQLState, mysqlErr.Message)
		} else {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		}
		r.logger.Log(query, 0, elapsed, err)
		return
	}

	affected, _ := result.RowsAffected()
	rowCount := int(affected)

	if r.opts.Timing {
		fmt.Fprintf(os.Stderr, "Query OK, %d row(s) affected (%.2f sec)\n", rowCount, elapsed.Seconds())
	} else {
		fmt.Fprintf(os.Stderr, "Query OK, %d row(s) affected\n", rowCount)
	}

	if r.opts.Timing && r.slowThresh > 0 && elapsed > r.slowThresh {
		fmt.Fprintf(os.Stderr, " \u26a0 slow query (>%.2fs)\n", r.slowThresh.Seconds())
	}

	r.logger.Log(query, rowCount, elapsed, nil)
}

func (r *REPL) handleMetaCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	parts := strings.SplitN(cmd, " ", 2)
	command := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch command {
	case "\\q", "\\exit", "\\quit":
		return io.EOF
	case "\\l":
		return r.execQueryWrapped("SHOW DATABASES")
	case "\\d":
		if arg != "" {
			if !isValidIdentifier(arg) {
				return fmt.Errorf("invalid table name: %q", arg)
			}
			return r.execQueryWrapped(fmt.Sprintf("DESCRIBE `%s`", escapeIdentifier(arg)))
		}
		return r.execQueryWrapped("SHOW TABLES")
	case "\\u", "\\use":
		if arg == "" {
			return fmt.Errorf("usage: \\u <database>")
		}
		if !isValidIdentifier(arg) {
			return fmt.Errorf("invalid database name: %q", arg)
		}
		return r.execQueryWrapped(fmt.Sprintf("USE `%s`", escapeIdentifier(arg)))
	case "\\s", "\\status":
		r.showStatus()
		return nil
	case "\\t":
		return r.switchFormat(arg)
	case "\\G":
		r.formatter = format.NewVerticalFormatter(os.Stdout)
		fmt.Fprintln(os.Stdout, "Output format switched to vertical.")
		return nil
	case "\\T":
		r.formatter = format.NewTableFormatter(os.Stdout, false)
		fmt.Fprintln(os.Stdout, "Output format switched to table.")
		return nil
	case "\\h", "\\help":
		r.showHelp()
		return nil
	case "\\timing":
		r.opts.Timing = !r.opts.Timing
		status := "disabled"
		if r.opts.Timing {
			status = "enabled"
		}
		fmt.Fprintf(os.Stdout, "Timing %s.\n", status)
		return nil
	case "\\c", "\\connect":
		return fmt.Errorf("reconnection is not yet supported in this version")
	case "\\i", "\\include":
		if arg == "" {
			return fmt.Errorf("usage: \\i <file>")
		}
		return r.execFile(arg)
	case "\\echo":
		fmt.Fprintln(os.Stdout, arg)
		return nil
	default:
		return fmt.Errorf("unknown command: %s, type \\h for help", command)
	}
}

func (r *REPL) execQueryWrapped(query string) error {
	rows, err := r.db.Query(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return nil
	}

	var resultRows [][]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			return nil
		}
		resultRows = append(resultRows, values)
	}

	r.formatter.FormatResult(os.Stdout, columns, resultRows, len(resultRows), 0)
	return nil
}

func (r *REPL) showStatus() {
	stats := r.db.Stats()
	fmt.Fprintf(os.Stdout, "Connection pool stats:\n")
	fmt.Fprintf(os.Stdout, "  Max open connections: %d\n", stats.MaxOpenConnections)
	fmt.Fprintf(os.Stdout, "  Open connections: %d\n", stats.OpenConnections)
	fmt.Fprintf(os.Stdout, "  In use: %d\n", stats.InUse)
	fmt.Fprintf(os.Stdout, "  Idle: %d\n", stats.Idle)
	fmt.Fprintf(os.Stdout, "  Format: %s\n", r.opts.Format)
	fmt.Fprintf(os.Stdout, "  Timing: %v\n", r.opts.Timing)
	return
}

func (r *REPL) switchFormat(arg string) error {
	supported := map[string]bool{
		"table": true, "csv": true, "json": true,
		"json-pretty": true, "json-rows": true, "tsv": true, "vertical": true,
	}
	if arg == "" || !supported[arg] {
		return fmt.Errorf("usage: \\t <format> (table, csv, json, json-pretty, json-rows, tsv, vertical)")
	}
	r.opts.Format = arg
	r.formatter = format.NewFormatter(arg, false, os.Stdout)
	fmt.Fprintf(os.Stdout, "Output format switched to %s.\n", arg)
	return nil
}

func (r *REPL) execFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var query strings.Builder
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		query.WriteString(line)
		query.WriteByte(' ')

		if strings.HasSuffix(trimmed, ";") {
			r.execQuery(strings.TrimSpace(query.String()))
			query.Reset()
		}
	}

	if query.Len() > 0 {
		remaining := strings.TrimSpace(query.String())
		if remaining != "" {
			r.execQuery(remaining)
		}
	}

	return nil
}

func (r *REPL) showHelp() {
	help := `Meta-commands:
  \q, \exit, \quit      Exit
  \c, \connect          Reconnect
  \d [table]            List tables or describe table
  \l                    List databases
  \u, \use <db>         Switch database
  \s, \status           Show connection status
  \t <format>           Switch output format (table/csv/json/tsv/vertical)
  \G                    Switch to vertical display
  \T                    Switch to table display
  \timing               Toggle query timing display
  \i, \include <file>   Execute SQL file
  \echo <text>          Print text
  \h, \help             Show this help
`
	fmt.Fprint(os.Stdout, help)
}

func isValidIdentifier(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func escapeIdentifier(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}
