package migrations

import (
	"context"
	"database/sql"
)

// itemsV1 and itemsV2 are the items table before and after this migration.
// SQLite cannot change a CHECK constraint, so the table is rebuilt.
const (
	itemsV1 = `CREATE TABLE items (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		category   TEXT NOT NULL CHECK (category IN ('music', 'audiobook')),
		title      TEXT NOT NULL,
		cover_url  TEXT NOT NULL DEFAULT '',
		service    TEXT NOT NULL,
		kind       TEXT NOT NULL,
		ref_id     TEXT NOT NULL,
		UNIQUE (profile_id, category, service, kind, ref_id)
	)`

	itemsV2 = `CREATE TABLE items (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		profile_id   INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
		category     TEXT NOT NULL CHECK (category IN ('music', 'audiobook', 'playlist')),
		title        TEXT NOT NULL,
		cover_url    TEXT NOT NULL DEFAULT '',
		artist       TEXT NOT NULL DEFAULT '',
		release_date TEXT NOT NULL DEFAULT '',
		service      TEXT NOT NULL,
		kind         TEXT NOT NULL,
		ref_id       TEXT NOT NULL,
		UNIQUE (profile_id, category, service, kind, ref_id)
	)`
)

// upArtistReleaseDatePlaylist adds the playlist category and the artist and
// release_date columns, and moves existing playlists out of "music". A
// database that already has the artist column was upgraded by an earlier
// build, before migrations were tracked, so there is nothing to do.
func upArtistReleaseDatePlaylist(ctx context.Context, tx *sql.Tx) error {
	if done, err := hasColumn(ctx, tx, "items", "artist"); err != nil || done {
		return err
	}
	return execAll(ctx, tx,
		`ALTER TABLE items RENAME TO items_old`,
		itemsV2,
		`INSERT INTO items (id, profile_id, category, title, cover_url, service, kind, ref_id)
		 SELECT id, profile_id,
		        CASE WHEN kind = 'playlist' AND category = 'music' THEN 'playlist' ELSE category END,
		        title, cover_url, service, kind, ref_id
		 FROM items_old`,
		`DROP TABLE items_old`,
	)
}

// downArtistReleaseDatePlaylist goes back to the original table. The artist and
// release date are lost, and playlists become music again.
func downArtistReleaseDatePlaylist(ctx context.Context, tx *sql.Tx) error {
	return execAll(ctx, tx,
		`ALTER TABLE items RENAME TO items_old`,
		itemsV1,
		`INSERT INTO items (id, profile_id, category, title, cover_url, service, kind, ref_id)
		 SELECT id, profile_id,
		        CASE WHEN category = 'playlist' THEN 'music' ELSE category END,
		        title, cover_url, service, kind, ref_id
		 FROM items_old`,
		`DROP TABLE items_old`,
	)
}
