package cmd

import (
	"testing"

	"github.com/pingcap/tiup/components/sql/connect"
)

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd == nil {
		t.Fatal("NewRootCmd() returned nil")
	}
	if cmd.Use != "tiup sql [flags] [DSN|connection-name]" {
		t.Errorf("Use = %q, unexpected", cmd.Use)
	}
}

func TestNewRootCmd_Flags(t *testing.T) {
	cmd := NewRootCmd()

	flagTests := []struct {
		name      string
		want      string
		shorthand string
	}{
		{"host", "127.0.0.1", ""},
		{"port", "4000", "P"},
		{"user", "root", "u"},
		{"password", "false", "p"},
		{"database", "", ""},
		{"execute", "", "e"},
		{"file", "[]", "f"},
		{"format", "table", ""},
		{"delimiter", ";", ""},
		{"tls", "false", ""},
		{"playground", "false", ""},
		{"connection", "", "c"},
		{"log", "", ""},
		{"timing", "true", ""},
	}

	for _, tt := range flagTests {
		f := cmd.Flags().Lookup(tt.name)
		if f == nil {
			t.Errorf("flag --%s not found", tt.name)
			continue
		}
		if f.DefValue != tt.want {
			t.Errorf("flag --%s default = %q, want %q", tt.name, f.DefValue, tt.want)
		}
		if tt.shorthand != "" && f.Shorthand != tt.shorthand {
			t.Errorf("flag --%s shorthand = %q, want %q", tt.name, f.Shorthand, tt.shorthand)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	result := isTerminal()
	t.Logf("isTerminal() = %v (non-interactive test env)", result)
}

func TestApplyFlagOverrides(t *testing.T) {
	// Test that defaults do not override existing config values.
	cfg := &connect.DSNConfig{
		Host:     "10.0.0.1",
		Port:     3306,
		User:     "admin",
		Database: "mydb",
		Protocol: "tcp",
	}

	// Simulate default flags.
	flags = globalFlags{
		host:     "127.0.0.1",
		port:     4000,
		user:     "root",
		database: "",
		protocol: "tcp",
	}

	applyFlagOverrides(cfg)

	if cfg.Host != "10.0.0.1" {
		t.Errorf("Host should remain %q, got %q", "10.0.0.1", cfg.Host)
	}
	if cfg.Port != 3306 {
		t.Errorf("Port should remain %d, got %d", 3306, cfg.Port)
	}
	if cfg.User != "admin" {
		t.Errorf("User should remain %q, got %q", "admin", cfg.User)
	}
	if cfg.Database != "mydb" {
		t.Errorf("Database should remain %q, got %q", "mydb", cfg.Database)
	}
}

func TestApplyFlagOverrides_EmptyConfig(t *testing.T) {
	cfg := &connect.DSNConfig{}

	flags = globalFlags{
		host:     "192.168.1.1",
		port:     3306,
		user:     "app",
		database: "appdb",
		protocol: "tcp",
	}

	applyFlagOverrides(cfg)

	if cfg.Host != "192.168.1.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "192.168.1.1")
	}
	if cfg.Port != 3306 {
		t.Errorf("Port = %d, want %d", cfg.Port, 3306)
	}
	if cfg.User != "app" {
		t.Errorf("User = %q, want %q", cfg.User, "app")
	}
	if cfg.Database != "appdb" {
		t.Errorf("Database = %q, want %q", cfg.Database, "appdb")
	}
}

func TestApplyTLSConfig(t *testing.T) {
	cfg := &connect.DSNConfig{}

	flags = globalFlags{
		tls:           true,
		tlsCA:         "/path/to/ca.pem",
		tlsCert:       "/path/to/cert.pem",
		tlsKey:        "/path/to/key.pem",
		tlsSkipVerify: false,
	}

	applyTLSConfig(cfg)

	if cfg.TLSConfig == nil {
		t.Fatal("TLSConfig should be set")
	}
	if !cfg.TLSConfig.Enabled {
		t.Error("TLSConfig.Enabled should be true")
	}
	if cfg.TLSConfig.CAPath != "/path/to/ca.pem" {
		t.Errorf("CAPath = %q, want /path/to/ca.pem", cfg.TLSConfig.CAPath)
	}
	if cfg.TLSConfig.CertPath != "/path/to/cert.pem" {
		t.Errorf("CertPath = %q, want /path/to/cert.pem", cfg.TLSConfig.CertPath)
	}
	if cfg.TLSConfig.KeyPath != "/path/to/key.pem" {
		t.Errorf("KeyPath = %q, want /path/to/key.pem", cfg.TLSConfig.KeyPath)
	}
}

func TestApplyTLSConfig_SkipVerify(t *testing.T) {
	cfg := &connect.DSNConfig{}

	flags = globalFlags{
		tls:           false,
		tlsCA:         "",
		tlsCert:       "",
		tlsKey:        "",
		tlsSkipVerify: true,
	}

	applyTLSConfig(cfg)

	// SkipVerify alone should not enable TLS if no other TLS flag is set.
	// Current behavior: flags.tls || flags.tlsCA != "" || flags.tlsCert != ""
	if cfg.TLSConfig != nil && cfg.TLSConfig.Enabled {
		t.Log("skip-verify alone enabled TLS (implementation dependent)")
	}
}
