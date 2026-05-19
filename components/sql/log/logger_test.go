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

package log

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestQueryLogger_Disabled(t *testing.T) {
	logger := NewQueryLogger("")
	logger.Log("SELECT 1", 1, 10*time.Millisecond, nil)
	logger.Close()
}

func TestQueryLogger_TempFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/query.log"

	logger := NewQueryLogger(logPath)
	logger.Log("SELECT 1", 1, 10*time.Millisecond, nil)
	logger.Log("INSERT INTO t VALUES (1)", 0, 5*time.Millisecond, nil)
	logger.Log("INVALID SQL", 0, 1*time.Millisecond, testError{})
	logger.Close()

	data, err := readFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(data, "SELECT 1") {
		t.Error("log should contain 'SELECT 1'")
	}
	if !strings.Contains(data, "INSERT INTO t VALUES (1)") {
		t.Error("log should contain INSERT statement")
	}
	if !strings.Contains(data, "[rows:1]") {
		t.Error("log should contain row count")
	}
	if !strings.Contains(data, "[ERROR:") {
		t.Error("log should contain error marker for failed query")
	}
	if !strings.Contains(data, "[conn:1]") {
		t.Error("log should contain connection sequence number")
	}
}

func TestSanitizeDSN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mysql://root:secret@127.0.0.1:4000/db", "***"},
		{"SELECT * FROM users", "SELECT * FROM users"},
		{"root:password@host:3306/db", "***"},
	}

	for _, tt := range tests {
		result := sanitizeDSN(tt.input)
		if tt.want == "***" && !strings.Contains(result, "***") {
			t.Errorf("sanitizeDSN(%q) = %q, should mask credentials", tt.input, result)
		}
		if tt.want != "***" && result != tt.input {
			t.Errorf("sanitizeDSN(%q) = %q, want %q", tt.input, result, tt.input)
		}
	}
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}

type testError struct{}

func (e testError) Error() string { return "test error" }
