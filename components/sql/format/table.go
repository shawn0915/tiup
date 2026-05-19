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
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// TableFormatter formats results as ASCII tables.
type TableFormatter struct {
	w        io.Writer
	noHeader bool
}

// NewTableFormatter creates a new table formatter.
func NewTableFormatter(w io.Writer, noHeader bool) *TableFormatter {
	return &TableFormatter{w: w, noHeader: noHeader}
}

func (f *TableFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *TableFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	if len(columns) == 0 && len(rows) == 0 {
		return nil
	}

	writer := w
	if writer == nil {
		writer = f.w
	}

	colWidths := make([]int, len(columns))
	for i, col := range columns {
		colWidths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range rows {
		for i, val := range row {
			s := formatValue(val)
			if i < len(colWidths) && utf8.RuneCountInString(s) > colWidths[i] {
				colWidths[i] = utf8.RuneCountInString(s)
			}
		}
	}

	separator := buildSeparator(colWidths)

	fmt.Fprintln(writer, separator)

	if !f.noHeader {
		fmt.Fprint(writer, "|")
		for i, col := range columns {
			fmt.Fprintf(writer, " %-*s |", colWidths[i], col)
		}
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, separator)
	}

	for _, row := range rows {
		fmt.Fprint(writer, "|")
		for i := range columns {
			val := ""
			if i < len(row) {
				val = formatValue(row[i])
			}
			fmt.Fprintf(writer, " %-*s |", colWidths[i], val)
		}
		fmt.Fprintln(writer)
	}

	fmt.Fprintln(writer, separator)

	if rowCount >= 0 {
		rowLabel := "rows"
		if rowCount == 1 {
			rowLabel = "row"
		}
		if duration > 0 {
			fmt.Fprintf(writer, "%d %s in set (%.2f sec)\n", rowCount, rowLabel, duration.Seconds())
		} else {
			fmt.Fprintf(writer, "%d %s in set\n", rowCount, rowLabel)
		}
	}

	return nil
}

func (f *TableFormatter) FormatError(w io.Writer, err error) error {
	fmt.Fprintf(w, "ERROR: %s\n", err.Error())
	return nil
}

func (f *TableFormatter) Flush() error {
	if flusher, ok := f.w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	return nil
}

func buildSeparator(widths []int) string {
	var buf strings.Builder
	buf.WriteByte('+')
	for _, w := range widths {
		buf.WriteByte('-')
		for range w {
			buf.WriteByte('-')
		}
		buf.WriteByte('-')
		buf.WriteByte('+')
	}
	return buf.String()
}

func formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case []byte:
		if utf8.Valid(val) {
			return string(val)
		}
		return fmt.Sprintf("<BINARY %d bytes>", len(val))
	case time.Time:
		return val.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", val)
	}
}
