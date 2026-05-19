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
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// CSVFormatter formats results as CSV.
type CSVFormatter struct {
	w        io.Writer
	noHeader bool
}

// NewCSVFormatter creates a new CSV formatter.
func NewCSVFormatter(w io.Writer, noHeader bool) *CSVFormatter {
	return &CSVFormatter{w: w, noHeader: noHeader}
}

func (f *CSVFormatter) SetWriter(w io.Writer) { f.w = w }

func (f *CSVFormatter) FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error {
	writer := w
	if writer == nil {
		writer = f.w
	}

	csvWriter := csv.NewWriter(writer)

	if !f.noHeader && len(columns) > 0 {
		if err := csvWriter.Write(columns); err != nil {
			return err
		}
	}

	for _, row := range rows {
		record := make([]string, len(row))
		for i, val := range row {
			record[i] = formatValue(val)
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}

	csvWriter.Flush()
	return csvWriter.Error()
}

func (f *CSVFormatter) FormatError(w io.Writer, err error) error {
	fmt.Fprintf(w, "ERROR: %s\n", err.Error())
	return nil
}

func (f *CSVFormatter) Flush() error {
	return nil
}
