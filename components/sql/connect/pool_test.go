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

package connect

import (
	"testing"
)

func TestOpenRaw_InvalidDSN(t *testing.T) {
	_, err := OpenRaw("invalid:dsn:without:proper:format")
	if err == nil {
		t.Error("OpenRaw with invalid DSN should fail")
	}
}

func TestDBInterface_Methods(t *testing.T) {
	// Ensure the DB interface matches *sql.DB expectations.
	var _ DB = (*sqlDB)(nil)
}

func TestSqlDB_WrapperMethods(t *testing.T) {
	// This test verifies the wrapper methods compile and exist.
	// Real connection testing requires a running database.
	db := &sqlDB{}
	_ = db.Ping
	_ = db.Stats
	_ = db.Close
}
