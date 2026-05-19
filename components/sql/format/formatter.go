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
	"io"
	"time"
)

// Formatter is the interface for output formatting.
type Formatter interface {
	FormatResult(w io.Writer, columns []string, rows [][]interface{}, rowCount int, duration time.Duration) error
	FormatError(w io.Writer, err error) error
	Flush() error
	SetWriter(w io.Writer)
}

// NewFormatter creates a new Formatter based on the format name.
func NewFormatter(formatName string, noHeader bool, w io.Writer) Formatter {
	switch formatName {
	case "csv":
		return NewCSVFormatter(w, noHeader)
	case "json":
		return NewJSONFormatter(w, false)
	case "json-pretty":
		return NewJSONFormatter(w, true)
	case "json-rows":
		return NewJSONRowsFormatter(w)
	case "tsv":
		return NewTSVFormatter(w, noHeader)
	case "vertical":
		return NewVerticalFormatter(w)
	case "table":
		fallthrough
	default:
		return NewTableFormatter(w, noHeader)
	}
}
