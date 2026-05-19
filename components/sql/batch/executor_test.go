// Copyright 2025 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on the "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package batch

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/pingcap/tiup/components/sql/format"
	"github.com/pingcap/tiup/components/sql/log"
)

// mockDB implements connect.DB for testing.
type mockDB struct {
	queries       []string
	queryResults  [][]any
	queryColumns  []string
	queryErr      error
	execResults   map[string]sql.Result
	execErr       error
}

func (m *mockDB) Exec(query string, args ...any) (sql.Result, error) {
	m.queries = append(m.queries, query)
	if m.execErr != nil {
		return nil, m.execErr
	}
	if r, ok := m.execResults[query]; ok {
		return r, nil
	}
	return &mockResult{rowsAffected: 1}, nil
}

func (m *mockDB) Query(query string, args ...any) (*sql.Rows, error) {
	m.queries = append(m.queries, query)
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	// Return nil rows; tests that need rows should use integration testing.
	return nil, fmt.Errorf("mock Query not fully implemented")
}

func (m *mockDB) QueryRow(query string, args ...any) *sql.Row {
	return nil
}

func (m *mockDB) Close() error  { return nil }
func (m *mockDB) Ping() error   { return nil }
func (m *mockDB) Stats() sql.DBStats { return sql.DBStats{} }

type mockResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r *mockResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r *mockResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func TestSplitStatements(t *testing.T) {
	e := &Executor{opts: ExecutorOptions{Delimiter: ";"}}

	tests := []struct {
		name      string
		input     string
		wantCount int
		wantFirst string
	}{
		{
			name:      "single statement",
			input:     "SELECT 1;",
			wantCount: 1,
			wantFirst: "SELECT 1",
		},
		{
			name:      "multiple statements",
			input:     "SELECT 1; SELECT 2;",
			wantCount: 2,
			wantFirst: "SELECT 1",
		},
		{
			name:      "no trailing semicolon",
			input:     "SELECT 1",
			wantCount: 1,
			wantFirst: "SELECT 1",
		},
		{
			name:      "empty",
			input:     "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := e.splitStatements(tt.input)
			if len(stmts) != tt.wantCount {
				t.Errorf("splitStatements(%q) returned %d statements, want %d", tt.input, len(stmts), tt.wantCount)
			}
			if len(stmts) > 0 && strings.TrimSpace(stmts[0]) != tt.wantFirst {
				t.Errorf("first statement = %q, want %q", stmts[0], tt.wantFirst)
			}
		})
	}
}

func TestReadStatements(t *testing.T) {
	e := &Executor{opts: ExecutorOptions{Delimiter: ";"}}

	tests := []struct {
		name      string
		input     string
		wantCount int
	}{
		{
			name:      "basic SQL file",
			input:     "SELECT 1;\nSELECT 2;\n",
			wantCount: 2,
		},
		{
			name: "SQL with comments",
			input: "-- this is a comment\nSELECT 1;\n-- another comment\nSELECT 2;\n",
			wantCount: 2,
		},
		{
			name:      "SQL with blank lines",
			input:     "\n\nSELECT 1;\n\nSELECT 2;\n\n",
			wantCount: 2,
		},
		{
			name: "multi-line statement",
			input: "SELECT id,\n  name,\n  age\nFROM users\nWHERE id > 10;\n",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts, err := e.readStatements(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("readStatements error: %v", err)
			}
			if len(stmts) != tt.wantCount {
				t.Errorf("readStatements returned %d statements, want %d", len(stmts), tt.wantCount)
			}
		})
	}
}

func TestReadStatements_SemicolonsInStrings(t *testing.T) {
	e := &Executor{opts: ExecutorOptions{Delimiter: ";"}}

	input := "INSERT INTO t VALUES ('hello;world');\nSELECT 1;\n"
	stmts, err := e.readStatements(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readStatements error: %v", err)
	}

	if len(stmts) != 2 {
		t.Errorf("expected 2 statements, got %d: %+v", len(stmts), stmts)
	}

	if len(stmts) >= 1 {
		first := strings.TrimSpace(stmts[0])
		if first != "INSERT INTO t VALUES ('hello;world')" {
			t.Errorf("first statement = %q, want %q", first, "INSERT INTO t VALUES ('hello;world')")
		}
	}
	if len(stmts) >= 2 {
		second := strings.TrimSpace(stmts[1])
		if second != "SELECT 1" {
			t.Errorf("second statement = %q, want %q", second, "SELECT 1")
		}
	}
}

func TestSplitStatements_SemicolonsInStrings(t *testing.T) {
	e := &Executor{opts: ExecutorOptions{Delimiter: ";"}}

	tests := []struct {
		name      string
		input     string
		wantCount int
		wantStmts []string
	}{
		{
			name:      "semicolon inside single quotes",
			input:     "INSERT INTO t VALUES ('a;b'); SELECT 1;",
			wantCount: 2,
			wantStmts: []string{"INSERT INTO t VALUES ('a;b')", "SELECT 1"},
		},
		{
			name:      "semicolon inside double quotes",
			input:     `INSERT INTO t VALUES ("x;y"); SELECT 2;`,
			wantCount: 2,
			wantStmts: []string{`INSERT INTO t VALUES ("x;y")`, "SELECT 2"},
		},
		{
			name:      "escaped single quote",
			input:     "INSERT INTO t VALUES ('it''s;ok'); SELECT 3;",
			wantCount: 2,
			wantStmts: []string{"INSERT INTO t VALUES ('it''s;ok')", "SELECT 3"},
		},
		{
			name:      "mixed quotes",
			input:     `SELECT "a;b", 'c;d' FROM t; DELETE FROM t;`,
			wantCount: 2,
			wantStmts: []string{`SELECT "a;b", 'c;d' FROM t`, "DELETE FROM t"},
		},
		{
			name:      "no semicolons in strings",
			input:     "SELECT 1; SELECT 2; SELECT 3;",
			wantCount: 3,
			wantStmts: []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stmts := e.splitStatements(tt.input)
			if len(stmts) != tt.wantCount {
				t.Errorf("splitStatements(%q) returned %d statements, want %d:\n%+v", tt.input, len(stmts), tt.wantCount, stmts)
				return
			}
			for i, want := range tt.wantStmts {
				if got := strings.TrimSpace(stmts[i]); got != want {
					t.Errorf("statement[%d] = %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestExecutor_ExecString_DryRun(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		DryRun: true,
		Timing: false,
	})

	err := executor.ExecString("SELECT 1; SELECT 2;")
	if err != nil {
		t.Fatalf("ExecString dry-run error: %v", err)
	}
	if len(db.queries) != 0 {
		t.Errorf("dry-run should not execute queries, got %d", len(db.queries))
	}
}

func TestExecutor_ExecString_DML(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		Timing: false,
	})

	err := executor.ExecString("INSERT INTO t VALUES (1); UPDATE t SET x = 2;")
	if err != nil {
		t.Fatalf("ExecString error: %v", err)
	}
	if len(db.queries) != 2 {
		t.Errorf("expected 2 executed queries, got %d", len(db.queries))
	}
}

func TestExecutor_ExecString_OnErrorStop(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{execErr: fmt.Errorf("mock exec error")}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		OnError: "stop",
		Timing:  false,
	})

	err := executor.ExecString("INSERT INTO t VALUES (1);")
	if err == nil {
		t.Fatal("expected error with on-error=stop")
	}
}

func TestExecutor_ExecString_OnErrorContinue(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{execErr: fmt.Errorf("mock exec error")}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		OnError: "continue",
		Timing:  false,
	})

	err := executor.ExecString("INSERT INTO t VALUES (1); INSERT INTO t VALUES (2);")
	if err != nil {
		t.Fatalf("unexpected error with on-error=continue: %v", err)
	}
}

func TestExecutor_ExecFiles_ContextCancel(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := executor.ExecFiles(ctx, []string{"/tmp/nonexistent.sql"})
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestExecutor_ExecStdin_Empty(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{})

	ctx := context.Background()
	err := executor.ExecStdin(ctx)
	if err != nil {
		t.Fatalf("ExecStdin with empty stdin should not error: %v", err)
	}
}

func TestExecutor_ExecString_Echo(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		Echo:   true,
		DryRun: true,
	})

	err := executor.ExecString("SELECT 1;")
	if err != nil {
		t.Fatalf("ExecString error: %v", err)
	}
}

func TestExecutor_ExecString_MaxRows(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		MaxRows: 5,
		DryRun:  true,
	})

	// Dry-run should not hit MaxRows logic, but ensure option is wired.
	err := executor.ExecString("SELECT 1;")
	if err != nil {
		t.Fatalf("ExecString error: %v", err)
	}
}

func TestNewExecutor_Defaults(t *testing.T) {
	db := &mockDB{}
	executor := NewExecutor(db, nil, nil, ExecutorOptions{})
	if executor.opts.Delimiter != ";" {
		t.Errorf("default delimiter = %q, want ;", executor.opts.Delimiter)
	}
	if executor.opts.OnError != "stop" {
		t.Errorf("default on-error = %q, want stop", executor.opts.OnError)
	}
}

func TestExecutor_ExecFile_NotFound(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{})

	ctx := context.Background()
	err := executor.ExecFiles(ctx, []string{"/tmp/does_not_exist_12345.sql"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExecutor_ExecDirectory(t *testing.T) {
	var buf bytes.Buffer
	db := &mockDB{}
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	executor := NewExecutor(db, formatter, logger, ExecutorOptions{
		DryRun: true,
	})

	ctx := context.Background()
	// Pass a directory path that does not exist; should error.
	err := executor.ExecFiles(ctx, []string{"/tmp/does_not_exist_12345/"})
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestExecutor_ExecString_MultipleDelimiters(t *testing.T) {
	e := &Executor{opts: ExecutorOptions{Delimiter: "GO"}}

	input := "SELECT 1GO SELECT 2GO"
	stmts := e.splitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("expected 2 statements for GO delimiter, got %d", len(stmts))
	}
}
