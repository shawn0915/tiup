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
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFile_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	restore := setTestHome(tmpDir)
	defer restore()

	configCache = nil

	err := SaveConnection("test-conn", &DSNConfig{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "testdb",
		Protocol: "tcp",
		Charset:  "utf8mb4",
		Params:   map[string]string{},
	})
	if err != nil {
		t.Fatalf("SaveConnection error: %v", err)
	}

	cfg, err := LoadConnection("test-conn")
	if err != nil {
		t.Fatalf("LoadConnection error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "127.0.0.1")
	}
	if cfg.Port != 4000 {
		t.Errorf("Port = %d, want %d", cfg.Port, 4000)
	}
	if cfg.Database != "testdb" {
		t.Errorf("Database = %q, want %q", cfg.Database, "testdb")
	}

	connections, err := ListConnections()
	if err != nil {
		t.Fatalf("ListConnections error: %v", err)
	}
	if len(connections) != 1 {
		t.Fatalf("ListConnections returned %d connections, want 1", len(connections))
	}
	if connections[0].Name != "test-conn" {
		t.Errorf("Connection name = %q, want %q", connections[0].Name, "test-conn")
	}

	err = DeleteConnection("test-conn")
	if err != nil {
		t.Fatalf("DeleteConnection error: %v", err)
	}

	_, err = LoadConnection("test-conn")
	if err == nil {
		t.Error("LoadConnection should fail after deletion")
	}
}

func TestConfigFile_InvalidName(t *testing.T) {
	err := SaveConnection("", &DSNConfig{Host: "127.0.0.1"})
	if err == nil {
		t.Error("SaveConnection should fail with empty name")
	}

	err = SaveConnection("test/conn", &DSNConfig{Host: "127.0.0.1"})
	if err == nil {
		t.Error("SaveConnection should fail with invalid characters in name")
	}
}

func TestConfigFile_FileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	restore := setTestHome(tmpDir)
	defer restore()
	configCache = nil

	_ = SaveConnection("check", &DSNConfig{
		Host: "127.0.0.1", Port: 4000, User: "root",
		Protocol: "tcp", Charset: "utf8mb4", Params: map[string]string{},
	})

	expectedPath := filepath.Join(tmpDir, ".tiup", configFileName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("config file should be created at %s", expectedPath)
	}
}

// setTestHome overrides HOME/USERPROFILE for testing and returns a restore function.
func setTestHome(dir string) func() {
	origHome, hadHome := os.LookupEnv("HOME")
	origUserProfile, hadUserProfile := os.LookupEnv("USERPROFILE")
	os.Setenv("HOME", dir)
	os.Setenv("USERPROFILE", dir)
	return func() {
		if hadHome {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		if hadUserProfile {
			os.Setenv("USERPROFILE", origUserProfile)
		} else {
			os.Unsetenv("USERPROFILE")
		}
	}
}
