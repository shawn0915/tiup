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
	"bufio"
	"fmt"
	"os"
	"path"

	"github.com/pingcap/tiup/pkg/localdata"
)

// DiscoverPlayground scans the TiUP data directory for running playground
// instances and returns a DSNConfig for the first available TiDB endpoint.
func DiscoverPlayground() (*DSNConfig, error) {
	tiupHome := os.Getenv(localdata.EnvNameHome)
	if tiupHome == "" {
		return nil, fmt.Errorf("TIUP_HOME not set, are you running outside of tiup?")
	}

	dataDir := path.Join(tiupHome, localdata.DataParentDir)
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dsnPath := path.Join(dataDir, entry.Name(), "dsn")
		dsns, err := readDSNFile(dsnPath)
		if err != nil {
			continue
		}
		if len(dsns) > 0 {
			return ParseDSN(dsns[0])
		}
	}

	return nil, fmt.Errorf("no running playground instances found, run 'tiup playground' to start one")
}

func readDSNFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var dsns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			dsns = append(dsns, line)
		}
	}
	return dsns, scanner.Err()
}
