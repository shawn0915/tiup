package repl

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pingcap/tiup/components/sql/format"
	"github.com/pingcap/tiup/components/sql/log"
)

// mockDB implements connect.DB for testing.
type mockDB struct {
	queries      []string
	queryColumns []string
	queryRows    [][]any
	queryErr     error
	execErr      error
	stats        sql.DBStats
}

func (m *mockDB) Exec(query string, args ...any) (sql.Result, error) {
	m.queries = append(m.queries, query)
	if m.execErr != nil {
		return nil, m.execErr
	}
	return &mockResult{rowsAffected: 1}, nil
}

func (m *mockDB) Query(query string, args ...any) (*sql.Rows, error) {
	m.queries = append(m.queries, query)
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return nil, fmt.Errorf("mock Query not fully implemented")
}

func (m *mockDB) QueryRow(query string, args ...any) *sql.Row { return nil }
func (m *mockDB) Close() error                                { return nil }
func (m *mockDB) Ping() error                                 { return nil }
func (m *mockDB) Stats() sql.DBStats                          { return m.stats }

type mockResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r *mockResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r *mockResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"users", true},
		{"my_table", true},
		{"db-name", true},
		{"TableName", true},
		{"t1", true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"table;name", false},
		{"drop table", false},
		{"'; DROP TABLE users;--", false},
		{"a.b", false},
		{"`inject`", false},
		{"normal_123", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidIdentifier(tt.input)
			if got != tt.want {
				t.Errorf("isValidIdentifier(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"users", "users"},
		{"table`name", "table``name"},
		{"normal", "normal"},
		{"`already`", "``already``"},
	}

	for _, tt := range tests {
		got := escapeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("escapeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNewREPL(t *testing.T) {
	r, err := New(nil, nil, nil, Options{Format: "table"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if r.prompt != "mysql> " {
		t.Errorf("prompt = %q, want %q", r.prompt, "mysql> ")
	}
}

func TestNewREPL_SlowThreshold(t *testing.T) {
	r, err := New(nil, nil, nil, Options{SlowThreshold: "500ms"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if r.slowThresh.Milliseconds() != 500 {
		t.Errorf("slowThresh = %v, want 500ms", r.slowThresh)
	}
}

func TestNewREPL_InvalidSlowThreshold(t *testing.T) {
	var buf bytes.Buffer
	// Capture stderr warning
	oldStderr := replTestStderr
	replTestStderr = &buf
	defer func() { replTestStderr = oldStderr }()

	r, err := New(nil, nil, nil, Options{SlowThreshold: "invalid"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if r.slowThresh != time.Second {
		t.Errorf("slowThresh should default to 1s for invalid input, got %v", r.slowThresh)
	}
}

var replTestStderr *bytes.Buffer

func TestReadQuery_SemicolonTerminated(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	input := "SELECT 1;\n"
	reader := bufio.NewReader(strings.NewReader(input))
	query, err := r.readQuery(reader)
	if err != nil {
		t.Fatalf("readQuery error: %v", err)
	}
	if strings.TrimSpace(query) != "SELECT 1;" {
		t.Errorf("query = %q, want %q", query, "SELECT 1;")
	}
}

func TestReadQuery_MetaCommand(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	input := "\\d\n"
	reader := bufio.NewReader(strings.NewReader(input))
	query, err := r.readQuery(reader)
	if err != nil {
		t.Fatalf("readQuery error: %v", err)
	}
	if query != "\\d" {
		t.Errorf("query = %q, want %q", query, "\\d")
	}
}

func TestHandleMetaCommand_Quit(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\q")
	if err == nil {
		t.Error("expected EOF for \\q")
	}
}

func TestHandleMetaCommand_Unknown(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\unknown")
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestHandleMetaCommand_Help(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\h")
	if err != nil {
		t.Errorf("expected no error for \\h, got: %v", err)
	}
}

func TestHandleMetaCommand_InvalidIdentifier(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\d table;drop")
	if err == nil {
		t.Error("expected error for invalid identifier")
	}
}

func TestHandleMetaCommand_UseNoArg(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\u")
	if err == nil {
		t.Error("expected error for \\u without argument")
	}
}

func TestHandleMetaCommand_Timing(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	if r.opts.Timing {
		t.Error("timing should be off by default in this test")
	}
	r.handleMetaCommand("\\timing")
	if !r.opts.Timing {
		t.Error("timing should be toggled on")
	}
	r.handleMetaCommand("\\timing")
	if r.opts.Timing {
		t.Error("timing should be toggled off")
	}
}

func TestHandleMetaCommand_SwitchFormat(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\t csv")
	if err != nil {
		t.Errorf("expected no error for \\t csv, got: %v", err)
	}
	if r.opts.Format != "csv" {
		t.Errorf("format = %q, want %q", r.opts.Format, "csv")
	}
}

func TestHandleMetaCommand_SwitchFormatInvalid(t *testing.T) {
	r, _ := New(nil, nil, nil, Options{})
	err := r.handleMetaCommand("\\t invalid")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestHandleMetaCommand_ListDatabases(t *testing.T) {
	db := &mockDB{}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{})
	err := r.handleMetaCommand("\\l")
	if err != nil {
		t.Errorf("expected no error for \\l, got: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "SHOW DATABASES") {
		t.Errorf("expected SHOW DATABASES query, got: %v", db.queries)
	}
}

func TestHandleMetaCommand_ListTables(t *testing.T) {
	db := &mockDB{}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{})
	err := r.handleMetaCommand("\\d")
	if err != nil {
		t.Errorf("expected no error for \\d, got: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "SHOW TABLES") {
		t.Errorf("expected SHOW TABLES query, got: %v", db.queries)
	}
}

func TestHandleMetaCommand_DescribeTable(t *testing.T) {
	db := &mockDB{}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{})
	err := r.handleMetaCommand("\\d users")
	if err != nil {
		t.Errorf("expected no error for \\d users, got: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "DESCRIBE") {
		t.Errorf("expected DESCRIBE query, got: %v", db.queries)
	}
}

func TestHandleMetaCommand_UseDatabase(t *testing.T) {
	db := &mockDB{}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{})
	err := r.handleMetaCommand("\\u testdb")
	if err != nil {
		t.Errorf("expected no error for \\u testdb, got: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "USE") {
		t.Errorf("expected USE query, got: %v", db.queries)
	}
}

func TestHandleMetaCommand_Status(t *testing.T) {
	db := &mockDB{stats: sql.DBStats{OpenConnections: 5}}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{})
	err := r.handleMetaCommand("\\s")
	if err != nil {
		t.Errorf("expected no error for \\s, got: %v", err)
	}
}

func TestHandleMetaCommand_Echo(t *testing.T) {
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(nil, formatter, nil, Options{})
	err := r.handleMetaCommand("\\echo hello world")
	if err != nil {
		t.Errorf("expected no error for \\echo, got: %v", err)
	}
}

func TestHandleMetaCommand_SwitchToVertical(t *testing.T) {
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(nil, formatter, nil, Options{})
	err := r.handleMetaCommand("\\G")
	if err != nil {
		t.Errorf("expected no error for \\G, got: %v", err)
	}
}

func TestHandleMetaCommand_SwitchToTable(t *testing.T) {
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(nil, formatter, nil, Options{})
	err := r.handleMetaCommand("\\T")
	if err != nil {
		t.Errorf("expected no error for \\T, got: %v", err)
	}
}

func TestHandleMetaCommand_Connect(t *testing.T) {
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(nil, formatter, nil, Options{})
	err := r.handleMetaCommand("\\c")
	if err == nil {
		t.Error("expected error for \\c (not supported)")
	}
}

func TestREPL_execDML(t *testing.T) {
	db := &mockDB{}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	r, _ := New(db, formatter, logger, Options{Timing: false})

	r.execQuery("INSERT INTO t VALUES (1)")
	if len(db.queries) != 1 {
		t.Errorf("expected 1 query, got %d", len(db.queries))
	}
}

func TestREPL_execDML_WithError(t *testing.T) {
	db := &mockDB{execErr: fmt.Errorf("mock error")}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	r, _ := New(db, formatter, logger, Options{Timing: false})

	r.execQuery("INSERT INTO t VALUES (1)")
	if len(db.queries) != 1 {
		t.Errorf("expected 1 query attempt, got %d", len(db.queries))
	}
}

func TestREPL_execSelect_WithError(t *testing.T) {
	db := &mockDB{queryErr: fmt.Errorf("mock query error")}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	logger := log.NewQueryLogger("")
	r, _ := New(db, formatter, logger, Options{Timing: false})

	r.execQuery("SELECT 1")
	if len(db.queries) != 1 {
		t.Errorf("expected 1 query attempt, got %d", len(db.queries))
	}
}

func TestREPL_showStatus_Output(t *testing.T) {
	db := &mockDB{stats: sql.DBStats{
		MaxOpenConnections: 10,
		OpenConnections:    5,
		InUse:              2,
		Idle:               3,
	}}
	var buf bytes.Buffer
	formatter := format.NewTableFormatter(&buf, false)
	r, _ := New(db, formatter, nil, Options{Format: "table", Timing: true})

	r.showStatus()
}

func TestNewFormatter_Creation(t *testing.T) {
	var buf bytes.Buffer
	f := format.NewFormatter("json-pretty", false, &buf)
	if f == nil {
		t.Fatal("NewFormatter returned nil")
	}
}
