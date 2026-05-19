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

package format

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTableFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	columns := []string{"id", "name", "age"}
	rows := [][]any{
		{1, "alice", 30},
		{2, "bob", 25},
	}

	err := f.FormatResult(nil, columns, rows, 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "alice") {
		t.Error("output should contain 'alice'")
	}
	if !strings.Contains(output, "bob") {
		t.Error("output should contain 'bob'")
	}
	if !strings.Contains(output, "2 rows in set") {
		t.Error("output should contain row count")
	}
}

func TestTableFormatter_NoHeader(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, true)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "| id |") {
		t.Error("output should not contain header row when noHeader=true")
	}
}

func TestTableFormatter_NilValue(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{{1, nil}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NULL") {
		t.Error("output should contain NULL for nil value")
	}
}

func TestTableFormatter_TimeValue(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	columns := []string{"created_at"}
	rows := [][]any{{
		time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
	}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "2026-05-18 10:00:00") {
		t.Errorf("output should contain formatted time, got: %s", output)
	}
}

func TestTableFormatter_BinaryValue(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	columns := []string{"data"}
	rows := [][]any{{[]byte{0xFF, 0xFE, 0xFD}}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "<BINARY 3 bytes>") {
		t.Errorf("output should contain binary placeholder, got: %s", output)
	}
}

func TestTableFormatter_UTF8ByteSlice(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	columns := []string{"text"}
	rows := [][]any{{[]byte("hello")}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("output should contain UTF-8 string from byte slice, got: %s", output)
	}
}

func TestTableFormatter_EmptyResult(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	err := f.FormatResult(nil, []string{}, [][]any{}, 0, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if output != "" {
		t.Errorf("expected empty output for empty result, got: %q", output)
	}
}

func TestTableFormatter_FormatError(t *testing.T) {
	var buf bytes.Buffer
	f := NewTableFormatter(&buf, false)

	err := f.FormatError(&buf, fmt.Errorf("test error"))
	if err != nil {
		t.Fatalf("FormatError error: %v", err)
	}
	if !strings.Contains(buf.String(), "ERROR") {
		t.Error("FormatError should contain ERROR prefix")
	}
}

func TestCSVFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewCSVFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{
		{1, "alice"},
		{2, "bob"},
	}

	err := f.FormatResult(nil, columns, rows, 2, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "id,name") {
		t.Error("output should contain CSV header")
	}
	if !strings.Contains(output, "1,alice") {
		t.Error("output should contain data row")
	}
}

func TestCSVFormatter_NoHeader(t *testing.T) {
	var buf bytes.Buffer
	f := NewCSVFormatter(&buf, true)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "id,name") {
		t.Error("output should not contain header when noHeader=true")
	}
}

func TestCSVFormatter_NilValue(t *testing.T) {
	var buf bytes.Buffer
	f := NewCSVFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{{1, nil}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NULL") {
		t.Errorf("output should contain NULL, got: %s", output)
	}
}

func TestJSONFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewJSONFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name":"alice"`) {
		t.Errorf("output should contain JSON data, got: %s", output)
	}
}

func TestJSONFormatter_PrettyOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewJSONFormatter(&buf, true)

	columns := []string{"id"}
	rows := [][]any{{1}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "  ") {
		t.Error("pretty JSON should contain indentation")
	}
}

func TestJSONFormatter_NilValue(t *testing.T) {
	var buf bytes.Buffer
	f := NewJSONFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{{1, nil}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name":null`) {
		t.Errorf("output should contain null for nil, got: %s", output)
	}
}

func TestJSONFormatter_Error(t *testing.T) {
	var buf bytes.Buffer
	f := NewJSONFormatter(&buf, false)

	err := f.FormatError(&buf, fmt.Errorf("test error"))
	if err != nil {
		t.Fatalf("FormatError error: %v", err)
	}
	if !strings.Contains(buf.String(), `"error"`) {
		t.Errorf("FormatError should contain error field, got: %s", buf.String())
	}
}

func TestJSONRowsFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewJSONRowsFormatter(&buf)

	columns := []string{"id", "name"}
	rows := [][]any{
		{1, "alice"},
		{2, "bob"},
	}

	err := f.FormatResult(nil, columns, rows, 2, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	lines := strings.Count(output, "\n")
	if lines != 2 {
		t.Errorf("expected 2 lines (NDJSON), got %d lines: %s", lines, output)
	}
}

func TestTSVFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewTSVFormatter(&buf, false)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "id\tname") {
		t.Error("output should contain TSV header")
	}
	if !strings.Contains(output, "1\talice") {
		t.Error("output should contain TSV data")
	}
}

func TestTSVFormatter_NoHeader(t *testing.T) {
	var buf bytes.Buffer
	f := NewTSVFormatter(&buf, true)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "id\tname") {
		t.Error("output should not contain header when noHeader=true")
	}
}

func TestVerticalFormatter_BasicOutput(t *testing.T) {
	var buf bytes.Buffer
	f := NewVerticalFormatter(&buf)

	columns := []string{"id", "name"}
	rows := [][]any{{1, "alice"}}

	err := f.FormatResult(nil, columns, rows, 1, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "id:") {
		t.Error("output should contain 'id:'")
	}
	if !strings.Contains(output, "name: alice") {
		t.Error("output should contain 'name: alice'")
	}
}

func TestVerticalFormatter_EmptyResult(t *testing.T) {
	var buf bytes.Buffer
	f := NewVerticalFormatter(&buf)

	err := f.FormatResult(nil, []string{"id"}, [][]any{}, 0, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Empty set") {
		t.Errorf("output should contain 'Empty set', got: %s", output)
	}
}

func TestVerticalFormatter_RowCountPlural(t *testing.T) {
	var buf bytes.Buffer
	f := NewVerticalFormatter(&buf)

	columns := []string{"id"}
	rows := [][]any{{1}, {2}}

	err := f.FormatResult(nil, columns, rows, 2, 0)
	if err != nil {
		t.Fatalf("FormatResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "2 rows in set") {
		t.Errorf("output should contain '2 rows in set', got: %s", output)
	}
}

func TestNewFormatter(t *testing.T) {
	var buf bytes.Buffer

	tests := []struct {
		format string
	}{
		{"table"},
		{"csv"},
		{"json"},
		{"json-pretty"},
		{"json-rows"},
		{"tsv"},
		{"vertical"},
		{"unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			f := NewFormatter(tt.format, false, &buf)
			if f == nil {
				t.Errorf("NewFormatter(%q) returned nil", tt.format)
			}
		})
	}
}

func TestFormatValue_Types(t *testing.T) {
	if got := formatValue(nil); got != "NULL" {
		t.Errorf("formatValue(nil) = %q, want NULL", got)
	}
	if got := formatValue(42); got != "42" {
		t.Errorf("formatValue(42) = %q, want 42", got)
	}
	if got := formatValue("hello"); got != "hello" {
		t.Errorf("formatValue(hello) = %q, want hello", got)
	}
	if got := formatValue([]byte("hello")); got != "hello" {
		t.Errorf("formatValue([]byte hello) = %q, want hello", got)
	}
	if got := formatValue([]byte{0xFF}); got != "<BINARY 1 bytes>" {
		t.Errorf("formatValue(binary) = %q, want <BINARY 1 bytes>", got)
	}
	tm := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	if got := formatValue(tm); got != "2026-05-18 10:00:00" {
		t.Errorf("formatValue(time) = %q, want 2026-05-18 10:00:00", got)
	}
}
