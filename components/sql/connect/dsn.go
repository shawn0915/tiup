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
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DSNConfig holds parsed connection parameters.
type DSNConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	Database     string
	Protocol     string
	Socket       string
	TLSConfig    *TLSConfig
	Timeout      time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Charset      string
	Collation    string
	Params       map[string]string
}

// TLSConfig holds TLS connection parameters.
type TLSConfig struct {
	Enabled    bool
	CAPath     string
	CertPath   string
	KeyPath    string
	SkipVerify bool
}

// ConnectionEntry represents a saved connection configuration.
type ConnectionEntry struct {
	Name     string     `yaml:"name"`
	Host     string     `yaml:"host"`
	Port     int        `yaml:"port"`
	User     string     `yaml:"user"`
	Database string     `yaml:"database"`
	Protocol string     `yaml:"protocol,omitempty"`
	Socket   string     `yaml:"socket,omitempty"`
	TLS      *TLSConfig `yaml:"tls,omitempty"`
}

// ConfigFile represents the connections configuration file structure.
type ConfigFile struct {
	Connections []ConnectionEntry `yaml:"connections"`
}

// ParseDSN parses a connection string in URL format or host:port format.
// Supports:
//
//	mysql://user:password@host:port/database?params
//	user:password@host:port/database
//	host:port
//	host
func ParseDSN(input string) (*DSNConfig, error) {
	cfg := &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   make(map[string]string),
		Timeout:  5 * time.Second,
	}

	if strings.Contains(input, "://") {
		return parseURL(input, cfg)
	}
	if strings.Contains(input, "@") {
		return parseUserHost(input, cfg)
	}
	return parseHostPort(input, cfg)
}

func parseURL(raw string, cfg *DSNConfig) (*DSNConfig, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			cfg.Password = pw
		}
	}

	host := u.Hostname()
	port := u.Port()

	if host != "" {
		cfg.Host = host
	}
	if port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid port '%s': %w", port, err)
		}
		cfg.Port = p
	}

	if len(u.Path) > 1 {
		cfg.Database = strings.TrimPrefix(u.Path, "/")
	}

	for k, v := range u.Query() {
		if len(v) > 0 {
			switch strings.ToLower(k) {
			case "timeout":
				if d, err := time.ParseDuration(v[0]); err == nil {
					cfg.Timeout = d
				}
			case "readtimeout":
				if d, err := time.ParseDuration(v[0]); err == nil {
					cfg.ReadTimeout = d
				}
			case "writetimeout":
				if d, err := time.ParseDuration(v[0]); err == nil {
					cfg.WriteTimeout = d
				}
			case "charset":
				cfg.Charset = v[0]
			case "collation":
				cfg.Collation = v[0]
			case "tls":
				if v[0] == "true" || v[0] == "skip-verify" {
					cfg.TLSConfig = &TLSConfig{Enabled: true, SkipVerify: v[0] == "skip-verify"}
				}
			default:
				cfg.Params[k] = v[0]
			}
		}
	}

	return cfg, nil
}

func parseUserHost(input string, cfg *DSNConfig) (*DSNConfig, error) {
	atIdx := strings.Index(input, "@")
	if atIdx < 0 {
		return nil, fmt.Errorf("invalid connection string: missing '@'")
	}

	userPart := input[:atIdx]
	hostPart := input[atIdx+1:]

	if colonIdx := strings.Index(userPart, ":"); colonIdx >= 0 {
		cfg.User = userPart[:colonIdx]
		cfg.Password = userPart[colonIdx+1:]
	} else {
		cfg.User = userPart
	}

	host, port, err := splitHostPort(hostPart)
	if err != nil {
		return nil, err
	}
	cfg.Host = host
	if port != 0 {
		cfg.Port = port
	}

	return cfg, nil
}

func parseHostPort(input string, cfg *DSNConfig) (*DSNConfig, error) {
	cleaned := strings.TrimPrefix(input, "/")
	if dbIdx := strings.Index(cleaned, "/"); dbIdx >= 0 {
		cfg.Database = cleaned[dbIdx+1:]
		cleaned = cleaned[:dbIdx]
	}

	if cleaned == "" {
		return cfg, nil
	}

	host, port, err := splitHostPort(cleaned)
	if err != nil {
		return nil, err
	}
	cfg.Host = host
	if port != 0 {
		cfg.Port = port
	}

	return cfg, nil
}

func splitHostPort(s string) (string, int, error) {
	host := s
	port := 0

	if strings.HasPrefix(s, "[") {
		cbIdx := strings.Index(s, "]")
		if cbIdx < 0 {
			return "", 0, fmt.Errorf("invalid host: missing ']'")
		}
		host = s[1:cbIdx]
		rest := s[cbIdx+1:]
		if strings.HasPrefix(rest, ":") {
			p, err := strconv.Atoi(rest[1:])
			if err != nil {
				return "", 0, fmt.Errorf("invalid port: %w", err)
			}
			port = p
		}
	} else if colonIdx := strings.LastIndex(s, ":"); colonIdx >= 0 {
		host = s[:colonIdx]
		p, err := strconv.Atoi(s[colonIdx+1:])
		if err != nil {
			return "", 0, fmt.Errorf("invalid port: %w", err)
		}
		port = p
	}

	return host, port, nil
}

// ToMySQLDSN converts the config to a go-sql-driver/mysql DSN string.
func (c *DSNConfig) ToMySQLDSN() string {
	var buf strings.Builder

	buf.WriteString(url.QueryEscape(c.User))
	if c.Password != "" {
		buf.WriteByte(':')
		buf.WriteString(url.QueryEscape(c.Password))
	}

	switch c.Protocol {
	case "unix":
		buf.WriteString("@unix(")
		buf.WriteString(c.Socket)
		buf.WriteString(")/")
	default:
		buf.WriteString("@tcp(")
		buf.WriteString(c.Host)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(c.Port))
		buf.WriteString(")/")
	}

	if c.Database != "" {
		buf.WriteString(c.Database)
	}

	params := []string{}
	if c.Charset != "" {
		params = append(params, "charset="+c.Charset)
	}
	if c.Collation != "" {
		params = append(params, "collation="+c.Collation)
	}
	if c.Timeout > 0 {
		params = append(params, "timeout="+c.Timeout.String())
	}
	if c.ReadTimeout > 0 {
		params = append(params, "readTimeout="+c.ReadTimeout.String())
	}
	if c.WriteTimeout > 0 {
		params = append(params, "writeTimeout="+c.WriteTimeout.String())
	}

	if c.TLSConfig != nil && c.TLSConfig.Enabled {
		params = append(params, "tls="+tlsDSNParam(c))
	}

	for k, v := range c.Params {
		params = append(params, k+"="+v)
	}

	if len(params) > 0 {
		buf.WriteByte('?')
		buf.WriteString(strings.Join(params, "&"))
	}

	return buf.String()
}

// SanitizedDSN returns a DSN string with the password masked.
func (c *DSNConfig) SanitizedDSN() string {
	sanitized := *c
	if sanitized.Password != "" {
		sanitized.Password = "***"
	}
	return sanitized.ToMySQLDSN()
}

// tlsDSNParam returns the TLS parameter value for the MySQL DSN.
// The actual TLS config registration is handled by SetupTLS() in tls.go.
func tlsDSNParam(c *DSNConfig) string {
	tc := c.TLSConfig
	if tc == nil || !tc.Enabled {
		return "false"
	}
	if tc.SkipVerify {
		return "skip-verify"
	}
	return fmt.Sprintf("tiup-sql-%s-%d", c.Host, c.Port)
}
