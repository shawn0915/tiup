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
)

// TSVFormatter formats results as tab-separated values.
type TSVFormatter struct {
	w        io.Writer
	noHeader bool
}

// NewTSVFormatter creates a new TSV formatter.
func NewTSVFormatter(w io.Writer, noHeader bool) *TSVFormatter {
	return &TSVFormatter{w: w, noHeader: noHeader}
}

func (f *TSVFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *TSVFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	writer := w
	if writer == nil {
		writer = f.w
	}

	if !f.noHeader && len(columns) > 0 {
		fmt.Fprintln(writer, strings.Join(columns, "\t"))
	}

	for _, row := range rows {
		vals := make([]string, len(row))
		for i, val := range row {
			vals[i] = formatValue(val)
		}
		fmt.Fprintln(writer, strings.Join(vals, "\t"))
	}

	return nil
}

func (f *TSVFormatter) FormatError(w io.Writer, err error) error {
	fmt.Fprintf(w, "ERROR: %s\n", err.Error())
	return nil
}

func (f *TSVFormatter) Flush() error { return nil }
