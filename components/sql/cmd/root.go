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

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pingcap/tiup/components/sql/batch"
	"github.com/pingcap/tiup/components/sql/connect"
	"github.com/pingcap/tiup/components/sql/format"
	"github.com/pingcap/tiup/components/sql/log"
	"github.com/pingcap/tiup/components/sql/repl"
	"github.com/spf13/cobra"
)

type globalFlags struct {
	host          string
	port          int
	user          string
	password      bool
	database      string
	socket        string
	protocol      string

	tls            bool
	tlsCA          string
	tlsCert        string
	tlsKey         string
	tlsSkipVerify  bool

	playground     bool
	clusterName    string
	component      string

	connectionName string
	saveConnection string
	listConnections bool
	deleteConnection string

	execute        string
	files          []string
	delimiter      string
	onError        string
	dryRun         bool
	echo           bool
	force          bool

	outputFormat   string
	noHeader       bool
	pager          string
	timing         bool
	slowThreshold  string

	logFile        string
}

var flags globalFlags

// NewRootCmd creates the root command for tiup sql.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "tiup sql [flags] [DSN|connection-name]",
		Short: "A universal SQL client for MySQL and TiDB",
		Long: `tiup sql is a general-purpose SQL client that supports connecting to
MySQL and TiDB databases. It provides interactive REPL, batch execution,
multiple output formats, and secure credential management.

Examples:
  tiup sql mysql://root:password@127.0.0.1:4000/test_db
  tiup sql --host 127.0.0.1 --port 4000 --user root test_db
  tiup sql --playground
  tiup sql -c local-dev`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSQL(cmd, args)
		},
	}

	bindConnectionFlags(rootCmd)
	bindTLSFlags(rootCmd)
	bindDiscoveryFlags(rootCmd)
	bindConnectionMgmtFlags(rootCmd)
	bindExecutionFlags(rootCmd)
	bindOutputFlags(rootCmd)
	bindLogFlags(rootCmd)

	return rootCmd
}

func bindConnectionFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.host, "host", "127.0.0.1", "Database host")
	cmd.Flags().IntVarP(&flags.port, "port", "P", 4000, "Database port")
	cmd.Flags().StringVarP(&flags.user, "user", "u", "root", "Database user")
	cmd.Flags().BoolVarP(&flags.password, "password", "p", false, "Prompt for password interactively")
	cmd.Flags().StringVar(&flags.database, "database", "", "Default database")
	cmd.Flags().StringVar(&flags.socket, "socket", "", "Unix socket path")
	cmd.Flags().StringVar(&flags.protocol, "protocol", "tcp", "Connection protocol: tcp, unix")
}

func bindTLSFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&flags.tls, "tls", false, "Enable TLS")
	cmd.Flags().StringVar(&flags.tlsCA, "tls-ca", "", "CA certificate file path")
	cmd.Flags().StringVar(&flags.tlsCert, "tls-cert", "", "Client certificate file path")
	cmd.Flags().StringVar(&flags.tlsKey, "tls-key", "", "Client private key file path")
	cmd.Flags().BoolVar(&flags.tlsSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification")
}

func bindDiscoveryFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&flags.playground, "playground", false, "Auto-discover and connect to local playground instance")
	cmd.Flags().StringVar(&flags.clusterName, "cluster", "", "Connect using TiUP cluster topology")
	cmd.Flags().StringVar(&flags.component, "component", "tidb", "Component type when using --cluster")
}

func bindConnectionMgmtFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.connectionName, "connection", "c", "", "Use a saved connection by name")
	cmd.Flags().StringVar(&flags.saveConnection, "save-connection", "", "Save current connection with this name")
	cmd.Flags().BoolVar(&flags.listConnections, "list-connections", false, "List all saved connections")
	cmd.Flags().StringVar(&flags.deleteConnection, "delete-connection", "", "Delete a saved connection by name")
}

func bindExecutionFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.execute, "execute", "e", "", "Execute SQL and exit (non-interactive mode)")
	cmd.Flags().StringArrayVarP(&flags.files, "file", "f", nil, "Execute SQL file(s) (non-interactive mode)")
	cmd.Flags().StringVar(&flags.delimiter, "delimiter", ";", "Statement delimiter")
	cmd.Flags().StringVar(&flags.onError, "on-error", "stop", "Error handling: stop, continue, abort")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Parse only, do not execute")
	cmd.Flags().BoolVar(&flags.echo, "echo", false, "Echo SQL before execution")
	cmd.Flags().BoolVar(&flags.force, "force", false, "Continue on connection errors")
}

func bindOutputFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.outputFormat, "format", "table", "Output format: table, csv, json, json-pretty, json-rows, tsv, vertical")
	cmd.Flags().BoolVar(&flags.noHeader, "no-header", false, "Suppress column headers")
	cmd.Flags().StringVar(&flags.pager, "pager", "", "Pager command (e.g. \"less -S\")")
	cmd.Flags().BoolVar(&flags.timing, "timing", true, "Show query execution time")
	cmd.Flags().StringVar(&flags.slowThreshold, "slow-threshold", "1s", "Slow query threshold")
}

func bindLogFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.logFile, "log", "", "SQL query log file path")
}

func runSQL(cmd *cobra.Command, args []string) error {
	if flags.listConnections {
		return listConnections()
	}
	if flags.deleteConnection != "" {
		return deleteConnection(flags.deleteConnection)
	}

	var dsnConfig *connect.DSNConfig
	var err error

	switch {
	case flags.playground:
		dsnConfig, err = connect.DiscoverPlayground()
		if err != nil {
			return fmt.Errorf("playground discovery failed: %w", err)
		}
	case flags.connectionName != "":
		dsnConfig, err = connect.LoadConnection(flags.connectionName)
		if err != nil {
			return fmt.Errorf("failed to load connection '%s': %w", flags.connectionName, err)
		}
	case len(args) > 0:
		dsnConfig, err = connect.ParseDSN(args[0])
		if err != nil {
			return fmt.Errorf("failed to parse DSN: %w", err)
		}
	default:
		dsnConfig = &connect.DSNConfig{
			Host:     flags.host,
			Port:     flags.port,
			User:     flags.user,
			Database: flags.database,
			Protocol: flags.protocol,
		}
	}

	applyFlagOverrides(dsnConfig)

	if dsnConfig.Password == "" {
		if envPW := os.Getenv("TIUP_SQL_PASSWORD"); envPW != "" {
			dsnConfig.Password = envPW
		} else {
			pw, err := readPassword()
			if err == nil && pw != "" {
				dsnConfig.Password = pw
			}
		}
	}

	applyTLSConfig(dsnConfig)

	if err := connect.SetupTLS(dsnConfig); err != nil {
		return fmt.Errorf("TLS setup failed: %w", err)
	}

	queryLogger := log.NewQueryLogger(flags.logFile)
	defer queryLogger.Close()

	db, err := connect.Open(dsnConfig)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer db.Close()

	if flags.saveConnection != "" {
		if err := connect.SaveConnection(flags.saveConnection, dsnConfig); err != nil {
			return fmt.Errorf("failed to save connection: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Connection '%s' saved.\n", flags.saveConnection)
	}

	formatter := format.NewFormatter(flags.outputFormat, flags.noHeader, os.Stdout)

	batchMode := flags.execute != "" || len(flags.files) > 0 || !isTerminal()

	if batchMode {
		return runBatch(cmd.Context(), db, formatter, queryLogger)
	}

	return runREPL(db, formatter, queryLogger)
}

func applyFlagOverrides(cfg *connect.DSNConfig) {
	if flags.host != "127.0.0.1" || cfg.Host == "" {
		cfg.Host = flags.host
	}
	if flags.port != 4000 || cfg.Port == 0 {
		cfg.Port = flags.port
	}
	if flags.user != "root" || cfg.User == "" {
		cfg.User = flags.user
	}
	if flags.database != "" {
		cfg.Database = flags.database
	}
	if flags.protocol != "tcp" {
		cfg.Protocol = flags.protocol
	}
	if flags.socket != "" {
		cfg.Socket = flags.socket
	}
}

func applyTLSConfig(cfg *connect.DSNConfig) {
	if flags.tls || flags.tlsCA != "" || flags.tlsCert != "" {
		cfg.TLSConfig = &connect.TLSConfig{
			Enabled:   true,
			CAPath:    flags.tlsCA,
			CertPath:  flags.tlsCert,
			KeyPath:   flags.tlsKey,
			SkipVerify: flags.tlsSkipVerify,
		}
	}
}

func runBatch(ctx context.Context, db connect.DB, formatter format.Formatter, logger *log.QueryLogger) error {
	executor := batch.NewExecutor(db, formatter, logger, batch.ExecutorOptions{
		Delimiter:  flags.delimiter,
		OnError:    flags.onError,
		DryRun:     flags.dryRun,
		Echo:       flags.echo,
		Timing:     flags.timing,
		MaxRows:    0,
	})

	if flags.execute != "" {
		return executor.ExecString(flags.execute)
	}

	if len(flags.files) > 0 {
		return executor.ExecFiles(ctx, flags.files)
	}

	return executor.ExecStdin(ctx)
}

func runREPL(db connect.DB, formatter format.Formatter, logger *log.QueryLogger) error {
	r, err := repl.New(db, formatter, logger, repl.Options{
		Format:       flags.outputFormat,
		Timing:       flags.timing,
		SlowThreshold: flags.slowThreshold,
		Pager:        flags.pager,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize REPL: %w", err)
	}
	return r.Run()
}

func listConnections() error {
	connections, err := connect.ListConnections()
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		fmt.Println("No saved connections.")
		return nil
	}
	fmt.Println("Saved connections:")
	for _, c := range connections {
		fmt.Printf("  %-20s  %s@%s:%d/%s\n", c.Name, c.User, c.Host, c.Port, c.Database)
	}
	return nil
}

func deleteConnection(name string) error {
	if err := connect.DeleteConnection(name); err != nil {
		return err
	}
	fmt.Printf("Connection '%s' deleted.\n", name)
	return nil
}

func readPassword() (string, error) {
	fmt.Fprintf(os.Stderr, "Enter password: ")
	return readPasswordSilent()
}

func readPasswordSilent() (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := setTerminalRaw(fd)
	if err != nil {
		return "", err
	}
	defer restoreTerminal(fd, oldState)

	var runes []rune
	for {
		var b [1]byte
		n, err := os.Stdin.Read(b[:])
		if err != nil || n == 0 {
			return "", err
		}
		if b[0] == '\r' || b[0] == '\n' {
			break
		}
		if b[0] == 3 { // Ctrl+C
			return "", io.EOF
		}
		if b[0] == 127 || b[0] == 8 { // Backspace / Delete
			if len(runes) > 0 {
				runes = runes[:len(runes)-1]
			}
			continue
		}
		runes = append(runes, rune(b[0]))
	}
	fmt.Fprintln(os.Stderr)
	return string(runes), nil
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
