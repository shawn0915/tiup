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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"
)

var (
	tlsMu   sync.Mutex
	tlsRegs = make(map[string]bool)
)

func safeTLSName(host string, port int) string {
	var safe strings.Builder
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			safe.WriteRune(r)
		}
	}
	return fmt.Sprintf("tiup-sql-%s-%d", safe.String(), port)
}

// RegisterTLSConfig registers a TLS configuration with the MySQL driver.
// This wraps mysql.RegisterTLSConfig to provide proper TLS functionality.
func RegisterTLSConfig(name string, config *tls.Config) error {
	tlsMu.Lock()
	defer tlsMu.Unlock()

	if err := mysql.RegisterTLSConfig(name, config); err != nil {
		return fmt.Errorf("failed to register TLS config '%s': %w", name, err)
	}
	tlsRegs[name] = true
	return nil
}

// SetupTLS builds and registers the TLS config for a DSNConfig.
// Must be called before ToMySQLDSN() if TLS is enabled.
func SetupTLS(c *DSNConfig) error {
	tc := c.TLSConfig
	if tc == nil || !tc.Enabled {
		return nil
	}
	if tc.SkipVerify {
		return nil
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: tc.SkipVerify,
	}

	if tc.CAPath != "" {
		caCert, err := os.ReadFile(tc.CAPath)
		if err != nil {
			return fmt.Errorf("failed to read CA certificate '%s': %w", tc.CAPath, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate from '%s'", tc.CAPath)
		}
		tlsCfg.RootCAs = caCertPool
	}

	if tc.CertPath != "" && tc.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(tc.CertPath, tc.KeyPath)
		if err != nil {
			return fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	name := safeTLSName(c.Host, c.Port)
	return RegisterTLSConfig(name, tlsCfg)
}
