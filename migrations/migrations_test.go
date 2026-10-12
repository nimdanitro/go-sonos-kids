package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func newDB(t *testing.T) *sql.DB {
	t.Helper()

	return openDB(t, filepath.Join(t.TempDir(), "test.db"))
}

func columns(t *testing.T, db *sql.DB, table string) string {
	t.Helper()

	rows, err := db.QueryContext(
		t.Context(),
		`SELECT name FROM pragma_table_info('`+table+`') ORDER BY cid`, //nolint:gosec // G202: table names come from the test itself
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string

	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}

		cols = append(cols, c)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return strings.Join(cols, ",")
}

func exec(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()

	for _, s := range stmts {
		if _, err := db.ExecContext(t.Context(), s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
}

const finalItemColumns = "id,profile_id,category,title,cover_url,artist,release_date,service,kind,ref_id,played_at,progress"

// The same round trip sitrep runs: up, down to nothing, up again.
func TestMigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)

	p, err := NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}

	if pending, err := Pending(ctx, db); err != nil || !pending {
		t.Fatalf("a new database has pending migrations: %v %v", pending, err)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}

	if pending, _ := Pending(ctx, db); pending {
		t.Error("nothing should be pending after up")
	}

	if got := columns(t, db, "items"); got != finalItemColumns {
		t.Errorf("items columns after up:\n got %s\nwant %s", got, finalItemColumns)
	}
	// The final schema accepts the new category, and the check still rejects others.
	exec(
		t,
		db,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, service, kind, ref_id) VALUES (1, 'playlist', 'P', 'tidal', 'playlist', 'p')`,
	)

	if _, err := db.ExecContext(
		t.Context(),
		`INSERT INTO items (profile_id, category, title, service, kind, ref_id) VALUES (1, 'podcast', 'x', 'tidal', 'album', 'x')`,
	); err == nil {
		t.Error("the category check should reject unknown categories")
	}

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down: %v", err)
	}

	var tables int

	_ = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('profiles', 'items')`).
		Scan(&tables)
	if tables != 0 {
		t.Errorf("%d tables left after migrating all the way down", tables)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up again: %v", err)
	}

	if got := columns(t, db, "items"); got != finalItemColumns {
		t.Errorf("items columns after the second up: %s", got)
	}

	if _, err := p.Up(ctx); err != nil { // applying nothing is fine
		t.Errorf("repeated up: %v", err)
	}
}

// Each step down undoes only its own change.
func TestDownStepsUndoOneChangeAtATime(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)

	p, _ := NewProvider(db)
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}

	exec(
		t,
		db,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, artist, release_date, played_at, progress, service, kind, ref_id)
		 VALUES (1, 'audiobook', 'Folge 1', 'Conni', '2008-03-01', '2026-10-12T20:00:00Z', '{"last":3}', 'spotify', 'album', '1')`,
	)

	steps := []struct{ name, columns string }{
		{"00004 progress", "id,profile_id,category,title,cover_url,artist,release_date,service,kind,ref_id,played_at"},
		{"00003 played_at", "id,profile_id,category,title,cover_url,artist,release_date,service,kind,ref_id"},
		{"00002 artist and release date", "id,profile_id,category,title,cover_url,service,kind,ref_id"},
	}
	for _, step := range steps {
		if _, err := p.Down(ctx); err != nil {
			t.Fatalf("down (%s): %v", step.name, err)
		}

		if got := columns(t, db, "items"); got != step.columns {
			t.Errorf("after undoing %s:\n got %s\nwant %s", step.name, got, step.columns)
		}
		// Whatever the step did not touch must survive it.
		var title string
		if err := db.QueryRowContext(t.Context(), `SELECT title FROM items`).
			Scan(&title); err != nil ||
			title != "Folge 1" {
			t.Errorf("the row must survive undoing %s: %q %v", step.name, title, err)
		}
	}
}

// Databases created before migrations were tracked have the tables but no
// goose bookkeeping. They must upgrade in place and keep every row.

func TestAdoptsDatabaseFromTheFirstRelease(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	exec(
		t,
		db,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, speaker TEXT NOT NULL, max_volume INTEGER NOT NULL DEFAULT 40, avatar TEXT NOT NULL DEFAULT '')`,
		itemsV1,
		`INSERT INTO profiles (name, speaker, max_volume) VALUES ('Globi', 'Globi', 35)`,
		`INSERT INTO items (profile_id, category, title, service, kind, ref_id) VALUES
			(1, 'music', 'An Album', 'tidal', 'album', '1'),
			(1, 'music', 'A Playlist', 'tidal', 'playlist', 'p'),
			(1, 'audiobook', 'A Story', 'spotify', 'album', '2')`,
	)

	if err := Up(ctx, db); err != nil {
		t.Fatalf("adopting a first-release database: %v", err)
	}

	if got := columns(t, db, "items"); got != finalItemColumns {
		t.Errorf("columns: %s", got)
	}

	cats := map[string]string{}

	rows, err := db.QueryContext(t.Context(), `SELECT title, category FROM items`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var title, cat string

		_ = rows.Scan(&title, &cat)
		cats[title] = cat
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(cats) != 3 || cats["An Album"] != "music" || cats["A Story"] != "audiobook" ||
		cats["A Playlist"] != "playlist" {
		t.Errorf("rows after adoption: %v", cats)
	}

	var vol int
	if err := db.QueryRowContext(t.Context(), `SELECT max_volume FROM profiles`).Scan(&vol); err != nil || vol != 35 {
		t.Errorf("profile lost: %d %v", vol, err)
	}
}

func TestAdoptsDatabaseWithArtistsButNoPlayedFlag(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	exec(
		t,
		db,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, speaker TEXT NOT NULL, max_volume INTEGER NOT NULL DEFAULT 40, avatar TEXT NOT NULL DEFAULT '')`,
		itemsV2,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, artist, release_date, service, kind, ref_id)
		 VALUES (1, 'music', 'Ruins', 'Jack and the Weatherman', '2026-09-25', 'tidal', 'album', '1')`,
		`PRAGMA user_version = 2`,
	) // as the pre-goose code left it

	if err := Up(ctx, db); err != nil {
		t.Fatalf("adopting a v2 database: %v", err)
	}

	var artist, date, played string
	if err := db.QueryRowContext(t.Context(), `SELECT artist, release_date, played_at FROM items`).
		Scan(&artist, &date, &played); err != nil {
		t.Fatal(err)
	}

	if artist != "Jack and the Weatherman" || date != "2026-09-25" || played != "" {
		t.Errorf("artist and date must be kept, played_at empty: %q %q %q", artist, date, played)
	}

	if got := columns(t, db, "items"); got != finalItemColumns {
		t.Errorf("columns: %s", got)
	}
}

func TestAdoptsDatabaseThatAlreadyHasEverything(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	exec(
		t,
		db,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, speaker TEXT NOT NULL, max_volume INTEGER NOT NULL DEFAULT 40, avatar TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE items (
			id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			category TEXT NOT NULL CHECK (category IN ('music', 'audiobook', 'playlist')), title TEXT NOT NULL, cover_url TEXT NOT NULL DEFAULT '',
			artist TEXT NOT NULL DEFAULT '', release_date TEXT NOT NULL DEFAULT '', played_at TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL, kind TEXT NOT NULL, ref_id TEXT NOT NULL, UNIQUE (profile_id, category, service, kind, ref_id))`,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, artist, played_at, service, kind, ref_id)
		 VALUES (1, 'audiobook', 'Folge 1', 'Conni', '2026-10-12T20:00:00Z', 'spotify', 'album', '1')`,
		`PRAGMA user_version = 3`,
	)

	if err := Up(ctx, db); err != nil {
		t.Fatalf("adopting a database that is already current: %v", err)
	}

	var played string
	if err := db.QueryRowContext(t.Context(), `SELECT played_at FROM items`).
		Scan(&played); err != nil ||
		played != "2026-10-12T20:00:00Z" {
		t.Errorf("played state lost: %q %v", played, err)
	}
}

// Asking whether migrations are pending must not change the database.
func TestPendingDoesNotTouchTheDatabase(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	exec(t, db, `CREATE TABLE profiles (id INTEGER PRIMARY KEY, name TEXT)`) // some pre-existing, untracked database

	if pending, err := Pending(ctx, db); err != nil || !pending {
		t.Fatalf("pending: %v %v", pending, err)
	}

	var tables int

	_ = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables)
	if tables != 1 {
		t.Errorf("Pending created tables: %d tables now, want 1", tables)
	}
}
