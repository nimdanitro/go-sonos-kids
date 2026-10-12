package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"

	"github.com/nimdanitro/go-sonos-kids/migrations"
	"github.com/nimdanitro/go-sonos-kids/store"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database schema migrations",
	Long: `Manage the database schema. "serve" applies pending migrations by itself on
start (see --migrate-on-startup); these commands are for doing it as a separate
step, checking where a database stands, or rolling a change back.`,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := dbPath(conf)
		if err != nil {
			return err
		}
		applied := 0
		err = migrateUp(cmd.Context(), path, func(r *goose.MigrationResult) {
			applied++
			fmt.Fprintf(cmd.OutOrStdout(), "applied  %05d  %-40s %s\n", r.Source.Version, filepath.Base(r.Source.Path), r.Duration.Round(time.Millisecond))
		})
		if err == nil && applied == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "the database is up to date")
		}
		return err
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Roll back the last applied migration (this can lose data)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, closeDB, err := openProvider()
		if err != nil {
			return err
		}
		defer closeDB()
		r, err := p.Down(cmd.Context())
		if errors.Is(err, goose.ErrNoNextVersion) {
			fmt.Fprintln(cmd.OutOrStdout(), "nothing to roll back")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "rolled back  %05d  %s\n", r.Source.Version, filepath.Base(r.Source.Path))
		return nil
	},
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show which migrations are applied",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, closeDB, err := openProvider()
		if err != nil {
			return err
		}
		defer closeDB()
		statuses, err := p.Status(cmd.Context())
		if err != nil {
			return err
		}
		for _, s := range statuses {
			state := "pending"
			if s.State == goose.StateApplied {
				state = "applied"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%5d  %-8s  %s\n", s.Source.Version, state, filepath.Base(s.Source.Path))
		}
		return nil
	},
}

// openProvider opens the configured database and returns a goose provider for it.
func openProvider() (*goose.Provider, func(), error) {
	path, err := dbPath(conf)
	if err != nil {
		return nil, nil, err
	}
	db, err := store.OpenDB(path)
	if err != nil {
		return nil, nil, err
	}
	p, err := migrations.NewProvider(db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return p, func() { db.Close() }, nil
}

// migrateUp applies the pending migrations to the database at path, reporting each one.
func migrateUp(ctx context.Context, path string, report func(*goose.MigrationResult)) error {
	db, err := store.OpenDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	p, err := migrations.NewProvider(db)
	if err != nil {
		return err
	}
	results, err := p.Up(ctx)
	for _, r := range results {
		report(r)
	}
	return err
}

func init() {
	registerDBFlag(migrateCmd.PersistentFlags())
	migrateCmd.AddCommand(migrateUpCmd, migrateDownCmd, migrateStatusCmd)
	rootCmd.AddCommand(migrateCmd)
}
