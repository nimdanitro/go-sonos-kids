// Package store is the SQLite-backed library: kid profiles and the albums,
// playlists and audiobooks curated for each of them.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, no cgo

	"github.com/nimdanitro/go-sonos-kids/migrations"
)

const (
	CategoryMusic     = "music"
	CategoryAudiobook = "audiobook"
	CategoryPlaylist  = "playlist"
)

// ValidCategory reports whether c is one of the known categories.
func ValidCategory(c string) bool {
	return c == CategoryMusic || c == CategoryAudiobook || c == CategoryPlaylist
}

var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("already exists")
)

// Profile is a kid's account, tied to the Sonos room they play on.
type Profile struct {
	ID        int64
	Name      string
	Speaker   string // Sonos room name (or IP)
	MaxVolume int    // 0-100, enforced by the server
	Avatar    string // emoji or image URL, shown by the UI
}

// Item is one album, playlist or track saved to a profile's library.
type Item struct {
	ID           int64
	ProfileID    int64
	Category     string // CategoryMusic, CategoryAudiobook or CategoryPlaylist
	Title        string
	CoverURL     string
	Artist       string // used to group music; may be empty
	ReleaseDate  string // YYYY-MM-DD, or empty when unknown
	PlayedAt     string // RFC3339 UTC time it was last started, or empty if never played
	ProgressJSON string // how far the kid got, as JSON (see Progress); empty if none
	Service      string // "tidal" or "spotify"
	Kind         string // "album", "playlist" or "track"
	RefID        string
}

type Store struct{ db *sql.DB }

// OpenDB opens the SQLite database at path without touching its schema. The
// migrate command uses it directly.
func OpenDB(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1) // tiny workload; avoids SQLITE_BUSY entirely

	return db, nil
}

// Open opens the database at path and applies any pending migrations (see the
// migrations package), creating it if it does not exist.
func Open(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}

	if err := migrations.Up(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// ErrPendingMigrations means the database schema is behind this build.
var ErrPendingMigrations = errors.New("the database has pending migrations; run `sonoskids migrate up`")

// OpenNoMigrate opens the database at path without changing its schema, and
// fails with ErrPendingMigrations if it is not up to date. Use it when
// migrations are run as a separate step.
func OpenNoMigrate(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}

	pending, err := migrations.Pending(context.Background(), db)
	if err == nil && pending {
		err = ErrPendingMigrations
	}

	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func mapErr(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed"):
		return ErrDuplicate
	}

	return err
}

// ---- profiles

func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, speaker, max_volume, avatar FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Profile

	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Speaker, &p.MaxVolume, &p.Avatar); err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

func (s *Store) GetProfile(ctx context.Context, id int64) (Profile, error) {
	var p Profile

	err := s.db.QueryRowContext(ctx, `SELECT id, name, speaker, max_volume, avatar FROM profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Speaker, &p.MaxVolume, &p.Avatar)

	return p, mapErr(err)
}

// CreateProfile inserts p and returns it with its ID set.
func (s *Store) CreateProfile(ctx context.Context, p Profile) (Profile, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO profiles (name, speaker, max_volume, avatar) VALUES (?, ?, ?, ?)`,
		p.Name, p.Speaker, p.MaxVolume, p.Avatar)
	if err != nil {
		return Profile{}, mapErr(err)
	}

	p.ID, _ = res.LastInsertId()

	return p, nil
}

func (s *Store) UpdateProfile(ctx context.Context, p Profile) error {
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE profiles SET name = ?, speaker = ?, max_volume = ?, avatar = ? WHERE id = ?`,
		p.Name,
		p.Speaker,
		p.MaxVolume,
		p.Avatar,
		p.ID,
	)

	return affected(res, err)
}

// DeleteProfile removes the profile and, by cascade, its library.
func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	return affected(res, err)
}

// ---- items

const itemCols = `id, profile_id, category, title, cover_url, artist, release_date, played_at, progress, service, kind, ref_id`

func scanItem(row interface{ Scan(...any) error }) (Item, error) {
	var it Item

	err := row.Scan(
		&it.ID,
		&it.ProfileID,
		&it.Category,
		&it.Title,
		&it.CoverURL,
		&it.Artist,
		&it.ReleaseDate,
		&it.PlayedAt,
		&it.ProgressJSON,
		&it.Service,
		&it.Kind,
		&it.RefID,
	)

	return it, err
}

// ListItems returns a profile's library in display order (see SortItems). An
// empty category returns all of them.
func (s *Store) ListItems(ctx context.Context, profileID int64, category string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+itemCols+` FROM items
		WHERE profile_id = ? AND (? = '' OR category = ?) ORDER BY id`, profileID, category, category)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Item

	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, it)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	SortItems(out)

	return out, nil
}

// SortItems orders a library for display: categories in a fixed order; music
// and audiobooks by artist (the series) and then release date, oldest first;
// playlists in the order they were added. Unknown dates and artists come last.
func SortItems(items []Item) {
	rank := map[string]int{CategoryMusic: 0, CategoryAudiobook: 1, CategoryPlaylist: 2}
	// before reports a < b where "" sorts after everything else.
	before := func(a, b string) (less, decided bool) {
		if (a == "") != (b == "") {
			return a != "", true
		}

		if a != b {
			return a < b, true
		}

		return false, false
	}

	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Category != b.Category {
			return rank[a.Category] < rank[b.Category]
		}

		if a.Category != CategoryPlaylist {
			if less, ok := before(strings.ToLower(a.Artist), strings.ToLower(b.Artist)); ok {
				return less
			}

			if less, ok := before(a.ReleaseDate, b.ReleaseDate); ok {
				return less
			}
		}

		return a.ID < b.ID
	})
}

// ListIncomplete returns albums and tracks, across all profiles, that lack an
// artist or a release date (e.g. saved before those were recorded). Playlists
// have neither, so they are not included.
func (s *Store) ListIncomplete(ctx context.Context) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+itemCols+` FROM items
		WHERE kind IN ('album', 'track') AND (artist = '' OR release_date = '') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Item

	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, it)
	}

	return out, rows.Err()
}

func (s *Store) GetItem(ctx context.Context, id int64) (Item, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE id = ?`, id))
	return it, mapErr(err)
}

// AddItem inserts it and returns it with its ID set. ErrDuplicate if the
// profile already has the same content in that category.
func (s *Store) AddItem(ctx context.Context, it Item) (Item, error) {
	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO items (profile_id, category, title, cover_url, artist, release_date, service, kind, ref_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ProfileID,
		it.Category,
		it.Title,
		it.CoverURL,
		it.Artist,
		it.ReleaseDate,
		it.Service,
		it.Kind,
		it.RefID,
	)
	if err != nil {
		return Item{}, mapErr(err)
	}

	it.ID, _ = res.LastInsertId()

	return it, nil
}

// UpdateItem changes the editable fields: category, title, cover, artist and date.
func (s *Store) UpdateItem(ctx context.Context, it Item) error {
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE items SET category = ?, title = ?, cover_url = ?, artist = ?, release_date = ? WHERE id = ?`,
		it.Category,
		it.Title,
		it.CoverURL,
		it.Artist,
		it.ReleaseDate,
		it.ID,
	)

	return affected(res, err)
}

// SetPlayed records that an item was just played (true), or forgets that it
// was (false), which also forgets how far the kid got.
func (s *Store) SetPlayed(ctx context.Context, id int64, played bool) error {
	if !played {
		res, err := s.db.ExecContext(ctx, `UPDATE items SET played_at = '', progress = '' WHERE id = ?`, id)
		return affected(res, err)
	}

	at := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE items SET played_at = ? WHERE id = ?`, at, id)

	return affected(res, err)
}

func (s *Store) DeleteItem(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	return affected(res, err)
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return mapErr(err)
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return nil
}
