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
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// DB is an interface abstracting *sql.DB for testability.
type DB interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Close() error
	Ping() error
	Stats() sql.DBStats
}

// sqlDB wraps *sql.DB to implement DB interface.
type sqlDB struct {
	*sql.DB
}

func (db *sqlDB) Ping() error {
	return db.DB.Ping()
}

func (db *sqlDB) Stats() sql.DBStats {
	return db.DB.Stats()
}

// Open creates a new database connection from a DSNConfig.
func Open(cfg *DSNConfig) (DB, error) {
	dsn := cfg.ToMySQLDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to %s:%d: %w", cfg.Host, cfg.Port, err)
	}

	return &sqlDB{db}, nil
}

// OpenRaw opens a connection from a raw MySQL DSN string.
func OpenRaw(dsn string) (DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &sqlDB{db}, nil
}
