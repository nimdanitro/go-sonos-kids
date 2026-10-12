package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProfilesAndItems(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	p, err := s.CreateProfile(ctx, Profile{Name: "Globi", Speaker: "Globi", MaxVolume: 35})
	if err != nil || p.ID == 0 {
		t.Fatalf("create profile: %+v %v", p, err)
	}
	if _, err := s.CreateProfile(ctx, Profile{Name: "Globi", Speaker: "x"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate profile: %v", err)
	}

	music, err := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryMusic, Title: "Bluey", Service: "tidal", Kind: "album", RefID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryAudiobook, Title: "Story", Service: "spotify", Kind: "album", RefID: "2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryMusic, Title: "Bluey again", Service: "tidal", Kind: "album", RefID: "1"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate item: %v", err)
	}
	if _, err := s.AddItem(ctx, Item{ProfileID: 999, Category: CategoryMusic, Title: "x", Service: "tidal", Kind: "album", RefID: "9"}); err == nil {
		t.Error("expected foreign key failure for unknown profile")
	}
	if _, err := s.AddItem(ctx, Item{ProfileID: p.ID, Category: "podcast", Title: "x", Service: "tidal", Kind: "album", RefID: "9"}); err == nil {
		t.Error("expected check failure for bad category")
	}

	if got, _ := s.ListItems(ctx, p.ID, ""); len(got) != 2 {
		t.Errorf("all items: %d", len(got))
	}
	if got, _ := s.ListItems(ctx, p.ID, CategoryMusic); len(got) != 1 || got[0].Title != "Bluey" {
		t.Errorf("music items: %+v", got)
	}

	music.Title = "Bluey Theme"
	if err := s.UpdateItem(ctx, music); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetItem(ctx, music.ID); got.Title != "Bluey Theme" {
		t.Errorf("after update: %+v", got)
	}

	if err := s.DeleteItem(ctx, music.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetItem(ctx, music.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted item: %v", err)
	}

	// Deleting a profile cascades to its library.
	if err := s.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListItems(ctx, p.ID, ""); len(got) != 0 {
		t.Errorf("items survived profile delete: %d", len(got))
	}
	if err := s.DeleteProfile(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing profile: %v", err)
	}
}

func TestMigrateFromV1(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v1.db")

	// A database as the first release created it: no artist/release_date
	// columns, and only two categories.
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, speaker TEXT NOT NULL, max_volume INTEGER NOT NULL DEFAULT 40, avatar TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			category TEXT NOT NULL CHECK (category IN ('music', 'audiobook')), title TEXT NOT NULL, cover_url TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL, kind TEXT NOT NULL, ref_id TEXT NOT NULL, UNIQUE (profile_id, category, service, kind, ref_id))`,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, service, kind, ref_id) VALUES
			(1, 'music', 'An Album', 'tidal', 'album', '1'),
			(1, 'music', 'A Playlist', 'tidal', 'playlist', 'p'),
			(1, 'audiobook', 'A Story', 'spotify', 'album', '2')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open v1 database: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	items, err := s.ListItems(ctx, 1, "")
	if err != nil || len(items) != 3 {
		t.Fatalf("items after migration: %+v %v", items, err)
	}
	cats := map[string]string{}
	for _, it := range items {
		cats[it.Title] = it.Category
	}
	if cats["An Album"] != CategoryMusic || cats["A Story"] != CategoryAudiobook || cats["A Playlist"] != CategoryPlaylist {
		t.Errorf("categories after migration: %v", cats)
	}
	// The new category and columns work, and reopening is a no-op.
	if _, err := s.AddItem(ctx, Item{ProfileID: 1, Category: CategoryPlaylist, Title: "x", Artist: "A", ReleaseDate: "2020-01-01", Service: "tidal", Kind: "playlist", RefID: "q"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got, _ := s2.ListItems(ctx, 1, ""); len(got) != 4 {
		t.Errorf("after reopen: %d items", len(got))
	}
}

func TestSortItems(t *testing.T) {
	items := []Item{
		{ID: 1, Category: CategoryPlaylist, Title: "second playlist"},
		{ID: 2, Category: CategoryAudiobook, Title: "conni 3", Artist: "Conni", ReleaseDate: "2021-05-01"},
		{ID: 3, Category: CategoryMusic, Title: "z", Artist: "Zebra", ReleaseDate: "2019-01-01"},
		{ID: 4, Category: CategoryAudiobook, Title: "bibi 2", Artist: "bibi blocksberg", ReleaseDate: "2020-03-01"},
		{ID: 5, Category: CategoryMusic, Title: "newer", Artist: "abba", ReleaseDate: "2010-01-01"},
		{ID: 6, Category: CategoryAudiobook, Title: "undated conni", Artist: "Conni"},
		{ID: 7, Category: CategoryMusic, Title: "older", Artist: "ABBA", ReleaseDate: "1980-01-01"},
		{ID: 8, Category: CategoryMusic, Title: "no artist", ReleaseDate: "2000-01-01"},
		{ID: 9, Category: CategoryAudiobook, Title: "conni 1", Artist: "Conni", ReleaseDate: "2008-01-01"},
		{ID: 10, Category: CategoryPlaylist, Title: "first playlist"},
		{ID: 11, Category: CategoryAudiobook, Title: "bibi 1", Artist: "Bibi Blocksberg", ReleaseDate: "2019-01-01"},
		{ID: 12, Category: CategoryAudiobook, Title: "no series", ReleaseDate: "1990-01-01"},
		{ID: 13, Category: CategoryAudiobook, Title: "conni 2", Artist: "Conni", ReleaseDate: "2010-01-01"},
	}
	SortItems(items)
	var got []string
	for _, it := range items {
		got = append(got, it.Title)
	}
	want := []string{
		"older", "newer", "z", "no artist", // music: artist case-insensitively, oldest first, unknown artist last
		"bibi 1", "bibi 2", // audiobooks: series (artist) first, even though Conni's episodes are older
		"conni 1", "conni 2", "conni 3", "undated conni", // ... each series oldest episode first, undated last
		"no series",                         // ... and items without an artist after all series
		"second playlist", "first playlist", // playlists: in the order they were added (by ID)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("order\n got %v\nwant %v", got, want)
	}
}

func TestListIncomplete(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.CreateProfile(ctx, Profile{Name: "A", Speaker: "A"})
	add := func(ref, kind, artist, date string) {
		t.Helper()
		if _, err := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryMusic, Title: ref, Artist: artist, ReleaseDate: date, Service: "tidal", Kind: kind, RefID: ref}); err != nil {
			t.Fatal(err)
		}
	}
	add("complete", "album", "Artist", "2020-01-01")
	add("no-artist", "album", "", "2020-01-01")
	add("no-date", "album", "Artist", "")
	add("track-neither", "track", "", "")
	add("playlist", "playlist", "", "") // never has either, so never "incomplete"

	got, err := s.ListIncomplete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, it := range got {
		refs = append(refs, it.RefID)
	}
	if strings.Join(refs, ",") != "no-artist,no-date,track-neither" {
		t.Errorf("incomplete: %v", refs)
	}
}

func TestPlayedFlag(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.CreateProfile(ctx, Profile{Name: "A", Speaker: "A"})
	a, _ := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryAudiobook, Title: "ep1", Service: "spotify", Kind: "album", RefID: "1"})
	b, _ := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryAudiobook, Title: "ep2", Service: "spotify", Kind: "album", RefID: "2"})

	if got, _ := s.GetItem(ctx, a.ID); got.PlayedAt != "" {
		t.Errorf("new items start unplayed: %q", got.PlayedAt)
	}
	if err := s.SetPlayed(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetItem(ctx, a.ID)
	if _, err := time.Parse(time.RFC3339, got.PlayedAt); err != nil {
		t.Errorf("played_at should be an RFC3339 time, got %q", got.PlayedAt)
	}
	if other, _ := s.GetItem(ctx, b.ID); other.PlayedAt != "" {
		t.Errorf("only the played item changes, got %q on the other", other.PlayedAt)
	}
	// Editing an item must not reset its played state.
	got.Title = "ep1 renamed"
	if err := s.UpdateItem(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.GetItem(ctx, a.ID); again.PlayedAt == "" || again.Title != "ep1 renamed" {
		t.Errorf("update lost the played state: %+v", again)
	}
	if err := s.SetPlayed(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}
	if cleared, _ := s.GetItem(ctx, a.ID); cleared.PlayedAt != "" {
		t.Errorf("not cleared: %q", cleared.PlayedAt)
	}
	if err := s.SetPlayed(ctx, 9999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown item: %v", err)
	}
}

func TestMigrateFromV2AddsPlayedAt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, speaker TEXT NOT NULL, max_volume INTEGER NOT NULL DEFAULT 40, avatar TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			category TEXT NOT NULL CHECK (category IN ('music', 'audiobook', 'playlist')), title TEXT NOT NULL, cover_url TEXT NOT NULL DEFAULT '',
			artist TEXT NOT NULL DEFAULT '', release_date TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL, kind TEXT NOT NULL, ref_id TEXT NOT NULL, UNIQUE (profile_id, category, service, kind, ref_id))`,
		`INSERT INTO profiles (name, speaker) VALUES ('Globi', 'Globi')`,
		`INSERT INTO items (profile_id, category, title, artist, release_date, service, kind, ref_id) VALUES (1, 'audiobook', 'Folge 1', 'Conni', '2008-03-01', 'spotify', 'album', '1')`,
		`PRAGMA user_version = 2`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open v2 database: %v", err)
	}
	defer s.Close()
	items, err := s.ListItems(ctx, 1, "")
	if err != nil || len(items) != 1 || items[0].Title != "Folge 1" || items[0].Artist != "Conni" || items[0].PlayedAt != "" {
		t.Fatalf("after migration: %+v %v", items, err)
	}
	if err := s.SetPlayed(ctx, items[0].ID, true); err != nil {
		t.Errorf("played_at column missing after migration: %v", err)
	}
}

func TestOpenNoMigrateRefusesAnOutdatedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")

	// A brand-new file has nothing applied yet.
	if s, err := OpenNoMigrate(path); !errors.Is(err, ErrPendingMigrations) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("empty database: %v", err)
	}
	// Refusing must not have changed anything.
	raw, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	var tables int
	_ = raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables)
	raw.Close()
	if tables != 0 {
		t.Errorf("a refused open created %d tables", tables)
	}

	// Once migrated (by Open, as `serve` does by default) it opens fine.
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(context.Background(), Profile{Name: "A", Speaker: "A"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := OpenNoMigrate(path)
	if err != nil {
		t.Fatalf("migrated database: %v", err)
	}
	defer s2.Close()
	if ps, _ := s2.ListProfiles(context.Background()); len(ps) != 1 {
		t.Errorf("profiles: %d", len(ps))
	}
}

func TestProgressRoundTripAndReset(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.CreateProfile(ctx, Profile{Name: "A", Speaker: "A"})
	it, _ := s.AddItem(ctx, Item{ProfileID: p.ID, Category: CategoryAudiobook, Title: "ep", Service: "spotify", Kind: "album", RefID: "1"})

	if _, ok := it.Progress(); ok {
		t.Error("a new item has no progress")
	}
	want := Progress{Last: 3, Total: 10, Tracks: map[int]TrackProgress{
		1: {Title: "Intro", Duration: 120, Position: 120, Done: true},
		2: {Title: "Kapitel 1", Duration: 600, Position: 598, Done: true},
		3: {Title: "Kapitel 2", Duration: 640, Position: 125},
	}}
	if err := s.SetProgress(ctx, it.ID, &want); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetItem(ctx, it.ID)
	back, ok := got.Progress()
	if !ok || back.Last != 3 || back.Total != 10 || back.Tracks[3].Position != 125 || back.Tracks[3].Title != "Kapitel 2" || back.TracksDone() != 2 {
		t.Errorf("round trip: %+v ok=%v (raw %s)", back, ok, got.ProgressJSON)
	}
	// It is real JSON, as stored: other tools can read it.
	var raw map[string]any
	if err := json.Unmarshal([]byte(got.ProgressJSON), &raw); err != nil || raw["last"] != float64(3) {
		t.Errorf("stored value is not the expected JSON: %s", got.ProgressJSON)
	}

	// Editing the item keeps the progress; marking it unplayed forgets it.
	got.Title = "renamed"
	if err := s.UpdateItem(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.GetItem(ctx, it.ID); again.ProgressJSON == "" {
		t.Error("editing an item must not reset its progress")
	}
	_ = s.SetPlayed(ctx, it.ID, true)
	if again, _ := s.GetItem(ctx, it.ID); again.ProgressJSON == "" {
		t.Error("marking an item played must not reset its progress")
	}
	if err := s.SetPlayed(ctx, it.ID, false); err != nil {
		t.Fatal(err)
	}
	if reset, _ := s.GetItem(ctx, it.ID); reset.ProgressJSON != "" || reset.PlayedAt != "" {
		t.Errorf("marking unplayed should forget everything: %+v", reset)
	}

	if err := s.SetProgress(ctx, it.ID, &want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProgress(ctx, it.ID, nil); err != nil {
		t.Fatal(err)
	}
	if cleared, _ := s.GetItem(ctx, it.ID); cleared.ProgressJSON != "" {
		t.Error("SetProgress(nil) should clear it")
	}
	if err := s.SetProgress(ctx, 9999, &want); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown item: %v", err)
	}
}

func TestParseProgressIgnoresJunk(t *testing.T) {
	for _, raw := range []string{"", "not json", "{}", `{"last":0}`, `{"last":-1}`} {
		if _, ok := ParseProgress(raw); ok {
			t.Errorf("%q should not count as progress", raw)
		}
	}
}
