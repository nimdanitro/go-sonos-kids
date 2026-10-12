// Package migrations holds the database schema migrations, applied with goose.
//
// SQL migrations are numbered files in this directory (00004_name.sql, with
// "-- +goose Up" and "-- +goose Down" sections). A migration that needs logic
// is written in Go and registered in GoMigrations.
//
// Databases created before migrations were tracked (they recorded a version in
// PRAGMA user_version) are adopted by migrations that check what already
// exists, so they upgrade without losing data.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// GoMigrations returns the migrations written in Go.
func GoMigrations() []*goose.Migration {
	artist := goose.NewGoMigration(2,
		&goose.GoFunc{RunTx: upArtistReleaseDatePlaylist},
		&goose.GoFunc{RunTx: downArtistReleaseDatePlaylist},
	)
	artist.Source = "00002_artist_release_date_playlist.go"

	played := goose.NewGoMigration(3,
		&goose.GoFunc{RunTx: upPlayedAt},
		&goose.GoFunc{RunTx: downPlayedAt},
	)
	played.Source = "00003_played_at.go"

	return []*goose.Migration{artist, played}
}

// NewProvider returns a goose provider for the SQLite database db.
func NewProvider(db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	opts = append([]goose.ProviderOption{goose.WithGoMigrations(GoMigrations()...)}, opts...)
	return goose.NewProvider(goose.DialectSQLite3, db, FS, opts...)
}

// Up applies every pending migration.
func Up(ctx context.Context, db *sql.DB) error {
	p, err := NewProvider(db)
	if err != nil {
		return err
	}

	_, err = p.Up(ctx)

	return err
}

// Pending reports whether there are migrations that have not been applied. It
// only looks: asking never changes the database, not even by creating goose's
// bookkeeping table.
func Pending(ctx context.Context, db *sql.DB) (bool, error) {
	var tracked int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&tracked); err != nil {
		return false, err
	}

	if tracked == 0 {
		return true, nil // nothing has ever been applied
	}

	p, err := NewProvider(db)
	if err != nil {
		return false, err
	}

	return p.HasPending(ctx)
}

// hasColumn reports whether table has a column of that name. Both are fixed
// identifiers in this package, never user input.
func hasColumn(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	var n int

	err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, table), column).Scan(&n)

	return n > 0, err
}

func execAll(ctx context.Context, tx *sql.Tx, statements ...string) error {
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
