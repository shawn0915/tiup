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
	"strings"
	"testing"
	"time"
)

func TestParseDSN_URLFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantUser string
		wantPass string
		wantDB   string
	}{
		{
			name:     "full URL",
			input:    "mysql://root:password@127.0.0.1:4000/test_db",
			wantHost: "127.0.0.1",
			wantPort: 4000,
			wantUser: "root",
			wantPass: "password",
			wantDB:   "test_db",
		},
		{
			name:     "URL without password",
			input:    "mysql://root@192.168.1.1:3306/prod",
			wantHost: "192.168.1.1",
			wantPort: 3306,
			wantUser: "root",
			wantPass: "",
			wantDB:   "prod",
		},
		{
			name:     "URL without database",
			input:    "mysql://admin:secret@10.0.0.1:4000/",
			wantHost: "10.0.0.1",
			wantPort: 4000,
			wantUser: "admin",
			wantPass: "secret",
			wantDB:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDSN(tt.input)
			if err != nil {
				t.Fatalf("ParseDSN(%q) error: %v", tt.input, err)
			}
			if cfg.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.wantHost)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.User != tt.wantUser {
				t.Errorf("User = %q, want %q", cfg.User, tt.wantUser)
			}
			if cfg.Password != tt.wantPass {
				t.Errorf("Password = %q, want %q", cfg.Password, tt.wantPass)
			}
			if cfg.Database != tt.wantDB {
				t.Errorf("Database = %q, want %q", cfg.Database, tt.wantDB)
			}
		})
	}
}

func TestParseDSN_UserHostFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantUser string
		wantPass string
	}{
		{
			name:     "user:pass@host:port",
			input:    "root:mypass@127.0.0.1:4000",
			wantHost: "127.0.0.1",
			wantPort: 4000,
			wantUser: "root",
			wantPass: "mypass",
		},
		{
			name:     "user@host:port",
			input:    "admin@db.example.com:3306",
			wantHost: "db.example.com",
			wantPort: 3306,
			wantUser: "admin",
			wantPass: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDSN(tt.input)
			if err != nil {
				t.Fatalf("ParseDSN(%q) error: %v", tt.input, err)
			}
			if cfg.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.wantHost)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
			if cfg.User != tt.wantUser {
				t.Errorf("User = %q, want %q", cfg.User, tt.wantUser)
			}
			if cfg.Password != tt.wantPass {
				t.Errorf("Password = %q, want %q", cfg.Password, tt.wantPass)
			}
		})
	}
}

func TestParseDSN_HostPortFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
	}{
		{
			name:     "host:port",
			input:    "127.0.0.1:4000",
			wantHost: "127.0.0.1",
			wantPort: 4000,
		},
		{
			name:     "host only",
			input:    "localhost",
			wantHost: "localhost",
			wantPort: 4000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseDSN(tt.input)
			if err != nil {
				t.Fatalf("ParseDSN(%q) error: %v", tt.input, err)
			}
			if cfg.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", cfg.Host, tt.wantHost)
			}
			if cfg.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.wantPort)
			}
		})
	}
}

func TestParseDSN_URLWithParams(t *testing.T) {
	cfg, err := ParseDSN("mysql://root:pw@127.0.0.1:4000/db?charset=utf8mb4&timeout=10s")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.Charset != "utf8mb4" {
		t.Errorf("Charset = %q, want %q", cfg.Charset, "utf8mb4")
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", cfg.Timeout)
	}
}

func TestParseDSN_WithReadWriteTimeout(t *testing.T) {
	cfg, err := ParseDSN("mysql://root@127.0.0.1:4000/db?readTimeout=5s&writeTimeout=3s")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want 5s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("WriteTimeout = %v, want 3s", cfg.WriteTimeout)
	}
}

func TestParseDSN_WithCollation(t *testing.T) {
	cfg, err := ParseDSN("mysql://root@127.0.0.1:4000/db?collation=utf8mb4_unicode_ci")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.Collation != "utf8mb4_unicode_ci" {
		t.Errorf("Collation = %q, want utf8mb4_unicode_ci", cfg.Collation)
	}
}

func TestParseDSN_WithTLSSkipVerify(t *testing.T) {
	cfg, err := ParseDSN("mysql://root@127.0.0.1:4000/db?tls=skip-verify")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.TLSConfig == nil || !cfg.TLSConfig.Enabled || !cfg.TLSConfig.SkipVerify {
		t.Error("expected TLS skip-verify config")
	}
}

func TestParseDSN_WithTLSTrue(t *testing.T) {
	cfg, err := ParseDSN("mysql://root@127.0.0.1:4000/db?tls=true")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.TLSConfig == nil || !cfg.TLSConfig.Enabled {
		t.Error("expected TLS enabled config")
	}
}

func TestParseDSN_Defaults(t *testing.T) {
	cfg, err := ParseDSN("")
	if err != nil {
		t.Fatalf("ParseDSN('') error: %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("default Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 4000 {
		t.Errorf("default Port = %d, want 4000", cfg.Port)
	}
	if cfg.User != "root" {
		t.Errorf("default User = %q, want root", cfg.User)
	}
	if cfg.Protocol != "tcp" {
		t.Errorf("default Protocol = %q, want tcp", cfg.Protocol)
	}
	if cfg.Charset != "utf8mb4" {
		t.Errorf("default Charset = %q, want utf8mb4", cfg.Charset)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("default Timeout = %v, want 5s", cfg.Timeout)
	}
}

func TestParseDSN_HostPortWithDatabase(t *testing.T) {
	cfg, err := ParseDSN("127.0.0.1:3306/mydb")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want 127.0.0.1", cfg.Host)
	}
	if cfg.Port != 3306 {
		t.Errorf("Port = %d, want 3306", cfg.Port)
	}
	if cfg.Database != "mydb" {
		t.Errorf("Database = %q, want mydb", cfg.Database)
	}
}

func TestParseDSN_IPV6(t *testing.T) {
	cfg, err := ParseDSN("[::1]:4000/test")
	if err != nil {
		t.Fatalf("ParseDSN error: %v", err)
	}
	if cfg.Host != "::1" {
		t.Errorf("Host = %q, want ::1", cfg.Host)
	}
	if cfg.Port != 4000 {
		t.Errorf("Port = %d, want 4000", cfg.Port)
	}
	if cfg.Database != "test" {
		t.Errorf("Database = %q, want test", cfg.Database)
	}
}

func TestParseDSN_InvalidPort(t *testing.T) {
	_, err := ParseDSN("127.0.0.1:abc")
	if err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestParseDSN_InvalidURL(t *testing.T) {
	_, err := ParseDSN("://invalid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestToMySQLDSN(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Password: "secret",
		Database: "test",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	dsn := cfg.ToMySQLDSN()
	if dsn == "" {
		t.Fatal("ToMySQLDSN returned empty string")
	}
	if !strings.Contains(dsn, "root:secret@tcp(127.0.0.1:4000)/test") {
		t.Errorf("unexpected DSN: %s", dsn)
	}
}

func TestToMySQLDSN_SpecialChars(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Password: "p@ss:w/rd",
		Database: "test",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	dsn := cfg.ToMySQLDSN()
	if strings.Contains(dsn, "p@ss:w/rd") && !strings.Contains(dsn, "p%40ss%3Aw%2Frd") {
		t.Errorf("password with special chars should be URL-encoded, got: %s", dsn)
	}
	if !strings.Contains(dsn, "p%40ss%3Aw%2Frd") {
		t.Errorf("DSN should contain URL-encoded password, got: %s", dsn)
	}
}

func TestToMySQLDSN_NoPassword(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "testdb",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	dsn := cfg.ToMySQLDSN()
	if dsn != "root@tcp(127.0.0.1:4000)/testdb?charset=utf8mb4" {
		t.Errorf("unexpected DSN: %s", dsn)
	}
}

func TestToMySQLDSN_WithParams(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "test",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{"parseTime": "true", "loc": "UTC"},
	}

	dsn := cfg.ToMySQLDSN()
	if !strings.Contains(dsn, "parseTime=true") {
		t.Errorf("DSN should contain parseTime param, got: %s", dsn)
	}
	if !strings.Contains(dsn, "loc=UTC") {
		t.Errorf("DSN should contain loc param, got: %s", dsn)
	}
}

func TestToMySQLDSN_WithTimeouts(t *testing.T) {
	cfg := &DSNConfig{
		Host:         "127.0.0.1",
		Port:         4000,
		User:         "root",
		Database:     "test",
		Protocol:     "tcp",
		Charset:      "utf8mb4",
		Timeout:      10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		Params:       map[string]string{},
	}

	dsn := cfg.ToMySQLDSN()
	if !strings.Contains(dsn, "timeout=10s") {
		t.Errorf("DSN should contain timeout, got: %s", dsn)
	}
	if !strings.Contains(dsn, "readTimeout=30s") {
		t.Errorf("DSN should contain readTimeout, got: %s", dsn)
	}
	if !strings.Contains(dsn, "writeTimeout=30s") {
		t.Errorf("DSN should contain writeTimeout, got: %s", dsn)
	}
}

func TestToMySQLDSN_ProtocolUnix(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "test",
		Protocol: "unix",
		Socket:   "/tmp/mysql.sock",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	dsn := cfg.ToMySQLDSN()
	if !strings.Contains(dsn, "@unix(/tmp/mysql.sock)/") {
		t.Errorf("unexpected DSN for unix protocol: %s", dsn)
	}
}

func TestSanitizedDSN(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Password: "super_secret",
		Database: "test",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	sanitized := cfg.SanitizedDSN()
	if sanitized == "" {
		t.Fatal("SanitizedDSN returned empty string")
	}
	if contains(sanitized, "super_secret") {
		t.Errorf("SanitizedDSN still contains the password: %s", sanitized)
	}
	// Password "***" is URL-encoded in DSN
	if !contains(sanitized, "%2A%2A%2A") && !contains(sanitized, "***") {
		t.Errorf("SanitizedDSN doesn't contain masked password: %s", sanitized)
	}
}

func TestSanitizedDSN_NoPassword(t *testing.T) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "test",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	}

	sanitized := cfg.SanitizedDSN()
	if strings.Contains(sanitized, "%2A%2A%2A") || strings.Contains(sanitized, ":***@") {
		t.Errorf("SanitizedDSN should not add fake password when none exists: %s", sanitized)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParseDSN_InvalidInput(t *testing.T) {
	_, err := ParseDSN("")
	if err != nil {
		t.Errorf("ParseDSN('') should succeed with defaults, got error: %v", err)
	}
}
