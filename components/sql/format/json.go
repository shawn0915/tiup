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
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// JSONFormatter formats results as JSON arrays.
type JSONFormatter struct {
	w      io.Writer
	pretty bool
}

// NewJSONFormatter creates a new JSON formatter.
func NewJSONFormatter(w io.Writer, pretty bool) *JSONFormatter {
	return &JSONFormatter{w: w, pretty: pretty}
}

func (f *JSONFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *JSONFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	writer := w
	if writer == nil {
		writer = f.w
	}

	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			if i < len(row) {
				rowMap[col] = convertJSONValue(row[i])
			}
		}
		result = append(result, rowMap)
	}

	var data []byte
	var err error
	if f.pretty {
		data, err = json.MarshalIndent(result, "", "  ")
	} else {
		data, err = json.Marshal(result)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}

func (f *JSONFormatter) FormatError(w io.Writer, err error) error {
	errObj := map[string]string{"error": err.Error()}
	data, _ := json.Marshal(errObj)
	data = append(data, '\n')
	_, writeErr := w.Write(data)
	return writeErr
}

func (f *JSONFormatter) Flush() error { return nil }

// JSONRowsFormatter formats results as NDJSON (one JSON object per line).
type JSONRowsFormatter struct {
	w io.Writer
}

// NewJSONRowsFormatter creates a new NDJSON formatter.
func NewJSONRowsFormatter(w io.Writer) *JSONRowsFormatter {
	return &JSONRowsFormatter{w: w}
}

func (f *JSONRowsFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *JSONRowsFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	writer := w
	if writer == nil {
		writer = f.w
	}

	for _, row := range rows {
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			if i < len(row) {
				rowMap[col] = convertJSONValue(row[i])
			}
		}
		data, err := json.Marshal(rowMap)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		data = append(data, '\n')
		if _, err := writer.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func (f *JSONRowsFormatter) FormatError(w io.Writer, err error) error {
	errObj := map[string]string{"error": err.Error()}
	data, _ := json.Marshal(errObj)
	data = append(data, '\n')
	_, writeErr := w.Write(data)
	return writeErr
}

func (f *JSONRowsFormatter) Flush() error { return nil }

func convertJSONValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		return string(val)
	case time.Time:
		return val.Format("2006-01-02 15:04:05")
	default:
		return val
	}
}
