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
	"time"
	"unicode/utf8"
)

// VerticalFormatter formats results in vertical (MySQL \G style) format.
type VerticalFormatter struct {
	w io.Writer
}

// NewVerticalFormatter creates a new vertical formatter.
func NewVerticalFormatter(w io.Writer) *VerticalFormatter {
	return &VerticalFormatter{w: w}
}

func (f *VerticalFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *VerticalFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	writer := w
	if writer == nil {
		writer = f.w
	}

	if len(rows) == 0 {
		fmt.Fprintf(writer, "Empty set\n")
		return nil
	}

	maxLabelLen := 0
	for _, c := range columns {
		if cl := utf8.RuneCountInString(c); cl > maxLabelLen {
			maxLabelLen = cl
		}
	}

	for i, row := range rows {
		separator := fmt.Sprintf("*************************** %d. row ***************************", i+1)
		fmt.Fprintln(writer, separator)

		for j, col := range columns {
			val := ""
			if j < len(row) {
				val = formatValue(row[j])
			}
			fmt.Fprintf(writer, "%*s: %s\n", maxLabelLen, col, val)
		}
	}

	rowLabel := "rows"
	if rowCount == 1 {
		rowLabel = "row"
	}
	if duration > 0 {
		fmt.Fprintf(writer, "%d %s in set (%.2f sec)\n", rowCount, rowLabel, duration.Seconds())
	} else {
		fmt.Fprintf(writer, "%d %s in set\n", rowCount, rowLabel)
	}

	return nil
}

func (f *VerticalFormatter) FormatError(w io.Writer, err error) error {
	fmt.Fprintf(w, "ERROR: %s\n", err.Error())
	return nil
}

func (f *VerticalFormatter) Flush() error { return nil }
