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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const configFileName = "connections.yaml"

var (
	configMu    sync.Mutex
	configCache *ConfigFile
)

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".tiup", configFileName), nil
}

func loadConfig() (*ConfigFile, error) {
	configMu.Lock()
	defer configMu.Unlock()

	if configCache != nil {
		return configCache, nil
	}

	p, err := configFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			configCache = &ConfigFile{Connections: []ConnectionEntry{}}
			return configCache, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	configCache = &cfg
	return configCache, nil
}

func saveConfig(cfg *ConfigFile) error {
	configMu.Lock()
	defer configMu.Unlock()

	p, err := configFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	configCache = cfg
	return nil
}

// SaveConnection saves a connection configuration by name.
func SaveConnection(name string, cfg *DSNConfig) error {
	if name == "" {
		return fmt.Errorf("connection name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return fmt.Errorf("connection name contains invalid characters")
	}

	config, err := loadConfig()
	if err != nil {
		return err
	}

	entry := ConnectionEntry{
		Name:     name,
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Database: cfg.Database,
		Protocol: cfg.Protocol,
		Socket:   cfg.Socket,
		TLS:      cfg.TLSConfig,
	}

	for i, existing := range config.Connections {
		if existing.Name == name {
			config.Connections[i] = entry
			return saveConfig(config)
		}
	}

	config.Connections = append(config.Connections, entry)
	return saveConfig(config)
}

// LoadConnection loads a saved connection by name.
func LoadConnection(name string) (*DSNConfig, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	for _, entry := range config.Connections {
		if entry.Name == name {
			return &DSNConfig{
				Host:      entry.Host,
				Port:      entry.Port,
				User:      entry.User,
				Database:  entry.Database,
				Protocol:  entry.Protocol,
				Socket:    entry.Socket,
				TLSConfig: entry.TLS,
				Charset:   "utf8mb4",
				Params:    make(map[string]string),
			}, nil
		}
	}

	return nil, fmt.Errorf("connection '%s' not found", name)
}

// DeleteConnection deletes a saved connection by name.
func DeleteConnection(name string) error {
	config, err := loadConfig()
	if err != nil {
		return err
	}

	for i, entry := range config.Connections {
		if entry.Name == name {
			config.Connections = append(config.Connections[:i], config.Connections[i+1:]...)
			return saveConfig(config)
		}
	}

	return fmt.Errorf("connection '%s' not found", name)
}

// ListConnections returns all saved connections sorted by name.
func ListConnections() ([]ConnectionEntry, error) {
	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	entries := make([]ConnectionEntry, len(config.Connections))
	copy(entries, config.Connections)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
