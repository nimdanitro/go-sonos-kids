package migrations

import (
	"context"
	"database/sql"
)

// upPlayedAt records when a kid last started an item. A database that already
// has the column was upgraded by an earlier build, so there is nothing to do.
func upPlayedAt(ctx context.Context, tx *sql.Tx) error {
	if done, err := hasColumn(ctx, tx, "items", "played_at"); err != nil || done {
		return err
	}

	return execAll(ctx, tx, `ALTER TABLE items ADD COLUMN played_at TEXT NOT NULL DEFAULT ''`)
}

func downPlayedAt(ctx context.Context, tx *sql.Tx) error {
	return execAll(ctx, tx, `ALTER TABLE items DROP COLUMN played_at`)
}
