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

package batch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pingcap/tiup/components/sql/connect"
	"github.com/pingcap/tiup/components/sql/format"
	"github.com/pingcap/tiup/components/sql/log"
)

// ExecutorOptions holds configuration for batch execution.
type ExecutorOptions struct {
	Delimiter  string
	OnError    string
	DryRun     bool
	Echo       bool
	Timing     bool
	MaxRows    int
}

// Executor handles batch SQL execution.
type Executor struct {
	db        connect.DB
	formatter format.Formatter
	logger    *log.QueryLogger
	opts      ExecutorOptions
}

// NewExecutor creates a new batch executor.
func NewExecutor(db connect.DB, formatter format.Formatter, logger *log.QueryLogger, opts ExecutorOptions) *Executor {
	if opts.Delimiter == "" {
		opts.Delimiter = ";"
	}
	if opts.OnError == "" {
		opts.OnError = "stop"
	}
	return &Executor{
		db:        db,
		formatter: formatter,
		logger:    logger,
		opts:      opts,
	}
}

// ExecString executes a single SQL string containing one or more statements.
func (e *Executor) ExecString(queryStr string) error {
	statements := e.splitStatements(queryStr)
	return e.execStatements(statements)
}

// ExecFiles executes SQL from one or more files.
func (e *Executor) ExecFiles(ctx context.Context, paths []string) error {
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("failed to stat '%s': %w", path, err)
		}

		if info.IsDir() {
			if err := e.execDirectory(ctx, path); err != nil {
				return err
			}
		} else {
			if err := e.execFile(path); err != nil {
				if e.opts.OnError == "stop" || e.opts.OnError == "abort" {
					return err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
		}
	}
	return nil
}

// ExecStdin reads and executes SQL from stdin.
func (e *Executor) ExecStdin(ctx context.Context) error {
	statements, err := e.readStatements(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read from stdin: %w", err)
	}
	return e.execStatements(statements)
}

func (e *Executor) execFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open '%s': %w", path, err)
	}
	defer file.Close()

	statements, err := e.readStatements(file)
	if err != nil {
		return fmt.Errorf("failed to read '%s': %w", path, err)
	}

	fmt.Fprintf(os.Stderr, "Executing %s (%d statements)\n", path, len(statements))
	return e.execStatements(statements)
}

func (e *Executor) execDirectory(ctx context.Context, dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory '%s': %w", dirPath, err)
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}

		filePath := dirPath + "/" + name
		if err := e.execFile(filePath); err != nil {
			if e.opts.OnError == "stop" || e.opts.OnError == "abort" {
				return err
			}
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	return nil
}

func (e *Executor) readStatements(r io.Reader) ([]string, error) {
	var statements []string
	var buf strings.Builder
	scanner := bufio.NewScanner(r)

	inSingleQuote := false
	inDoubleQuote := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inSingleQuote && !inDoubleQuote && (trimmed == "" || strings.HasPrefix(trimmed, "--")) {
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')

		if !inSingleQuote && !inDoubleQuote && strings.HasSuffix(trimmed, e.opts.Delimiter) {
			content := buf.String()
			content = strings.TrimSuffix(content, e.opts.Delimiter)
			content = strings.TrimSpace(content)

			reconstructed := e.splitStatements(content)
			if len(reconstructed) > 0 {
				stmt := strings.Join(reconstructed, "; ")
				statements = append(statements, stmt)
			}
			buf.Reset()
			inSingleQuote = false
			inDoubleQuote = false
			continue
		}

		for i := 0; i < len(line); i++ {
			ch := line[i]
			if ch == '\'' && !inDoubleQuote {
				if inSingleQuote && i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = !inSingleQuote
			} else if ch == '"' && !inSingleQuote {
				if inDoubleQuote && i+1 < len(line) && line[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = !inDoubleQuote
			}
		}
	}

	if buf.Len() > 0 {
		content := buf.String()
		content = strings.TrimSuffix(content, e.opts.Delimiter)
		content = strings.TrimSpace(content)
		if content != "" {
			reconstructed := e.splitStatements(content)
			for _, stmt := range reconstructed {
				statements = append(statements, stmt)
			}
		}
	}

	return statements, scanner.Err()
}

func (e *Executor) splitStatements(input string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if ch == '\'' && !inDoubleQuote {
			if inSingleQuote && i+1 < len(input) && input[i+1] == '\'' {
				current.WriteByte(ch)
				current.WriteByte(ch)
				i++
				continue
			}
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
			continue
		}

		if ch == '"' && !inSingleQuote {
			if inDoubleQuote && i+1 < len(input) && input[i+1] == '"' {
				current.WriteByte(ch)
				current.WriteByte(ch)
				i++
				continue
			}
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
			continue
		}

		if !inSingleQuote && !inDoubleQuote && strings.HasPrefix(input[i:], e.opts.Delimiter) {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			i += len(e.opts.Delimiter) - 1
			continue
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

func (e *Executor) execStatements(statements []string) error {
	var successes, failures int
	for i, stmt := range statements {
		if e.opts.Echo {
			fmt.Fprintf(os.Stderr, "-- Statement %d:\n%s;\n\n", i+1, stmt)
		}

		if e.opts.DryRun {
			fmt.Fprintf(os.Stderr, "[DRY RUN] %s;\n", stmt)
			continue
		}

		if err := e.execOne(stmt); err != nil {
			failures++
			switch e.opts.OnError {
			case "stop":
				return fmt.Errorf("statement %d failed: %w", i+1, err)
			case "abort":
				return fmt.Errorf("statement %d failed (abort): %w", i+1, err)
			case "continue":
				fmt.Fprintf(os.Stderr, "Warning: statement %d failed: %v\n", i+1, err)
			}
		} else {
			successes++
		}
	}

	fmt.Fprintf(os.Stderr, "Done: %d succeeded, %d failed\n", successes, failures)
	return nil
}

func (e *Executor) execOne(query string) error {
	start := time.Now()

	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	isQuery := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "EXPLAIN")

	if isQuery {
		return e.execQuery(query, start)
	}
	return e.execExec(query, start)
}

func (e *Executor) execQuery(query string, start time.Time) error {
	rows, err := e.db.Query(query)
	elapsed := time.Since(start)

	if err != nil {
		e.logger.Log(query, 0, elapsed, err)
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			return fmt.Errorf("ERROR %d (%s): %s", mysqlErr.Number, mysqlErr.SQLState, mysqlErr.Message)
		}
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	var resultRows [][]interface{}
	for rows.Next() {
		if e.opts.MaxRows > 0 && len(resultRows) >= e.opts.MaxRows {
			break
		}
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}
		resultRows = append(resultRows, values)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	rowCount := len(resultRows)
	e.logger.Log(query, rowCount, elapsed, nil)

	return e.formatter.FormatResult(nil, columns, resultRows, rowCount, elapsed)
}

func (e *Executor) execExec(query string, start time.Time) error {
	result, err := e.db.Exec(query)
	elapsed := time.Since(start)

	if err != nil {
		e.logger.Log(query, 0, elapsed, err)
		if mysqlErr, ok := err.(*mysql.MySQLError); ok {
			return fmt.Errorf("ERROR %d (%s): %s", mysqlErr.Number, mysqlErr.SQLState, mysqlErr.Message)
		}
		return err
	}

	affected, _ := result.RowsAffected()
	rowCount := int(affected)
	e.logger.Log(query, rowCount, elapsed, nil)

	if e.opts.Timing {
		fmt.Fprintf(os.Stderr, "Query OK, %d row(s) affected (%.2f sec)\n", rowCount, elapsed.Seconds())
	} else {
		fmt.Fprintf(os.Stderr, "Query OK, %d row(s) affected\n", rowCount)
	}
	return nil
}
