package connect

import (
	"testing"
)

func TestSetupTLS_Disabled(t *testing.T) {
	cfg := &DSNConfig{
		Host: "127.0.0.1",
		Port: 4000,
	}
	err := SetupTLS(cfg)
	if err != nil {
		t.Errorf("SetupTLS with no TLS config should not error, got: %v", err)
	}
}

func TestSetupTLS_NilTLSConfig(t *testing.T) {
	cfg := &DSNConfig{
		Host:      "127.0.0.1",
		Port:      4000,
		TLSConfig: nil,
	}
	err := SetupTLS(cfg)
	if err != nil {
		t.Errorf("SetupTLS with nil TLS config should not error, got: %v", err)
	}
}

func TestSetupTLS_SkipVerify(t *testing.T) {
	cfg := &DSNConfig{
		Host: "127.0.0.1",
		Port: 4000,
		TLSConfig: &TLSConfig{
			Enabled:    true,
			SkipVerify: true,
		},
	}
	err := SetupTLS(cfg)
	if err != nil {
		t.Errorf("SetupTLS with skip-verify should not error, got: %v", err)
	}
}

func TestSetupTLS_MissingCA(t *testing.T) {
	cfg := &DSNConfig{
		Host: "127.0.0.1",
		Port: 4000,
		TLSConfig: &TLSConfig{
			Enabled: true,
			CAPath:  "/nonexistent/ca.pem",
		},
	}
	err := SetupTLS(cfg)
	if err == nil {
		t.Error("expected error for missing CA certificate")
	}
}

func TestSetupTLS_MissingClientCert(t *testing.T) {
	cfg := &DSNConfig{
		Host: "127.0.0.1",
		Port: 4000,
		TLSConfig: &TLSConfig{
			Enabled:  true,
			CertPath: "/nonexistent/client-cert.pem",
			KeyPath:  "/nonexistent/client-key.pem",
		},
	}
	err := SetupTLS(cfg)
	if err == nil {
		t.Error("expected error for missing client certificate")
	}
}

func TestSafeTLSName(t *testing.T) {
	tests := []struct {
		host     string
		port     int
		expected string
	}{
		{"127.0.0.1", 4000, "tiup-sql-127.0.0.1-4000"},
		{"db.example.com", 3306, "tiup-sql-db.example.com-3306"},
		{"db_host-01", 4000, "tiup-sql-db_host-01-4000"},
		{"db@host!", 4000, "tiup-sql-dbhost-4000"},
		{"", 3306, "tiup-sql--3306"},
	}

	for _, tt := range tests {
		got := safeTLSName(tt.host, tt.port)
		if got != tt.expected {
			t.Errorf("safeTLSName(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.expected)
		}
	}
}

func TestTLSDSNParam(t *testing.T) {
	cfg := &DSNConfig{
		Host: "127.0.0.1",
		Port: 4000,
	}
	if got := tlsDSNParam(cfg); got != "false" {
		t.Errorf("tlsDSNParam with no TLS = %q, want false", got)
	}

	cfg.TLSConfig = &TLSConfig{Enabled: true, SkipVerify: true}
	if got := tlsDSNParam(cfg); got != "skip-verify" {
		t.Errorf("tlsDSNParam skip-verify = %q, want skip-verify", got)
	}

	cfg.TLSConfig = &TLSConfig{Enabled: true}
	if got := tlsDSNParam(cfg); got != "tiup-sql-127.0.0.1-4000" {
		t.Errorf("tlsDSNParam enabled = %q, want tiup-sql-127.0.0.1-4000", got)
	}
}
