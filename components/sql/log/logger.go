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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// QueryLogger records executed SQL queries to a log file.
type QueryLogger struct {
	mu       sync.Mutex
	writer   io.WriteCloser
	filePath string
	connSeq  int
}

// nopWriteCloser wraps an io.Writer to implement io.WriteCloser.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// NewQueryLogger creates a new query logger. If path is empty, logging is disabled.
func NewQueryLogger(path string) *QueryLogger {
	if path == "" {
		return &QueryLogger{writer: nopWriteCloser{io.Discard}}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open query log file '%s': %v\n", path, err)
		return &QueryLogger{writer: nopWriteCloser{io.Discard}}
	}

	return &QueryLogger{
		writer:   file,
		filePath: path,
	}
}

// Log records a query execution.
func (l *QueryLogger) Log(query string, rows int, duration time.Duration, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.connSeq++

	var errStr string
	if err != nil {
		errStr = fmt.Sprintf(" [ERROR: %v]", sanitizeError(err))
	}

	line := fmt.Sprintf(
		"[%s] [conn:%d] [timing:%s] [rows:%d]%s %s\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		l.connSeq,
		duration.Round(time.Microsecond),
		rows,
		errStr,
		sanitizeDSN(query),
	)

	_, _ = l.writer.Write([]byte(line))
}

// Close flushes and closes the log file.
func (l *QueryLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = l.writer.Close()
}

// sanitizeDSN removes potential credentials from log output.
func sanitizeDSN(s string) string {
	result := s

	if strings.Contains(result, "://") {
		result = redactBetween(result, "://", "@")
	}

	if strings.Contains(result, "@") && !strings.Contains(result, "://") {
		atIdx := strings.Index(result, "@")
		colonIdx := strings.Index(result, ":")
		if colonIdx >= 0 && atIdx > colonIdx {
			return result[:colonIdx+1] + "***" + result[atIdx:]
		}
	}

	return result
}

func redactBetween(s, start, end string) string {
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		return s
	}

	userPassStart := startIdx + len(start)
	endIdx := strings.Index(s[userPassStart:], end)
	if endIdx < 0 {
		return s
	}
	endIdx += userPassStart

	return s[:userPassStart] + "***" + s[endIdx:]
}

func sanitizeError(err error) string {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > 200 {
		return msg[:200] + "..."
	}
	return msg
}
