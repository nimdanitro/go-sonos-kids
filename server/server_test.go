package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nimdanitro/go-sonos-kids/catalog"
	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

type fakePlayer struct {
	calls   []string
	resumes []*sonos.Resume // what each Play was asked to resume from (nil = from the start)
	volume  int
	err     error
	status  PlayerStatus // what Status reports; State defaults to PLAYING
}

func (f *fakePlayer) Play(_ context.Context, room string, ref sonos.Ref, maxVol int, resume *sonos.Resume) error {
	f.calls = append(f.calls, "play "+room+" "+ref.String()+" max="+itoa(maxVol))
	f.resumes = append(f.resumes, resume)

	return f.err
}

func (f *fakePlayer) Control(_ context.Context, room, action string) error {
	f.calls = append(f.calls, action+" "+room)
	return f.err
}

func (f *fakePlayer) SetVolume(_ context.Context, room string, level int) error {
	f.volume = level
	f.calls = append(f.calls, "volume "+room+" "+itoa(level))

	return f.err
}

func (f *fakePlayer) Status(context.Context, string) (PlayerStatus, error) {
	if f.status.State == "" && f.status.Title == "" {
		return PlayerStatus{State: "PLAYING", Title: "Song", Volume: 20}, f.err
	}

	return f.status, f.err
}

func (f *fakePlayer) Rooms(context.Context) ([]RoomInfo, error) {
	return []RoomInfo{{"Globi", "Era 100"}}, nil
}

type fakeMeta struct{ info meta.Info }

func (f fakeMeta) Fetch(context.Context, sonos.Ref) (meta.Info, error) {
	if f.info.Title == "" {
		return meta.Info{}, errors.New("offline")
	}

	return f.info, nil
}

func itoa(i int) string { return strconv.Itoa(i) }

type env struct {
	t      *testing.T
	h      http.Handler
	player *fakePlayer
}

func newEnv(t *testing.T, token string, info meta.Info) *env {
	t.Helper()
	return newEnvWith(t, token, info, nil)
}

type fakeCatalog struct {
	d   catalog.Discography
	err error
}

func (f fakeCatalog) Discography(context.Context, string) (catalog.Discography, error) {
	return f.d, f.err
}

func newEnvWith(t *testing.T, token string, info meta.Info, cat Catalog) *env {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	p := &fakePlayer{}

	return &env{
		t,
		New(Options{Store: st, Player: p, Meta: fakeMeta{info}, Catalog: cat, AdminToken: token}).Handler(),
		p,
	}
}

func (e *env) do(method, path, token string, body any) (int, map[string]any, []byte) {
	e.t.Helper()

	var rdr *bytes.Reader

	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}

		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}

	req := httptest.NewRequestWithContext(e.t.Context(), method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)

	var m map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &m)

	return rec.Code, m, rec.Body.Bytes()
}

func (e *env) expect(want int, method, path, token string, body any) (map[string]any, []byte) {
	e.t.Helper()

	code, m, raw := e.do(method, path, token, body)
	if code != want {
		e.t.Fatalf("%s %s: got %d, want %d: %s", method, path, code, want, raw)
	}

	return m, raw
}

func TestAdminAccess(t *testing.T) {
	off := newEnv(t, "", meta.Info{})
	off.expect(503, "GET", "/api/admin/profiles", "anything", nil)

	e := newEnv(t, "secret", meta.Info{})
	e.expect(401, "GET", "/api/admin/profiles", "", nil)
	e.expect(401, "GET", "/api/admin/profiles", "wrong", nil)
	e.expect(200, "GET", "/api/admin/profiles", "secret", nil)
}

func TestLibraryAndPlayback(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{Title: "Kids Rock", CoverURL: "https://img/k.jpg"})

	globi, _ := e.expect(
		201,
		"POST",
		"/api/admin/profiles",
		"secret",
		map[string]any{"name": "Globi", "speaker": "Globi", "max_volume": 30, "avatar": "🦖"},
	)
	globine, _ := e.expect(
		201,
		"POST",
		"/api/admin/profiles",
		"secret",
		map[string]any{"name": "Globine", "speaker": "Globine"},
	)
	hID, lID := int(globi["id"].(float64)), int(globine["id"].(float64))

	e.expect(409, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "x"})
	e.expect(400, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "", "speaker": "x"})

	// Add one Tidal album to both kids as music; title/cover come from the lookup.
	res, _ := e.expect(201, "POST", "/api/admin/items", "secret", map[string]any{
		"url": "https://tidal.com/album/76466264", "category": "music", "profile_ids": []int{hID, lID},
	})
	if n := len(res["items"].([]any)); n != 2 {
		t.Fatalf("created %d items", n)
	}
	// Same link again: duplicate for both -> 409 with per-profile errors.
	res, _ = e.expect(409, "POST", "/api/admin/items", "secret", map[string]any{
		"url": "https://tidal.com/album/76466264", "category": "music", "profile_ids": []int{hID, lID},
	})
	if len(res["errors"].([]any)) != 2 {
		t.Errorf("errors: %v", res["errors"])
	}
	// Audiobook for Globi only, with a Spotify link and an explicit title.
	e.expect(201, "POST", "/api/admin/items", "secret", map[string]any{
		"url":         "spotify:album:6TDpQggB88X230Srv1fkSq",
		"category":    "audiobook",
		"profile_ids": []int{hID},
		"title":       "Zugpferdli",
	})
	e.expect(
		400,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "https://example.com/x", "category": "music", "profile_ids": []int{hID}},
	)
	e.expect(
		400,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "76466264", "category": "podcast", "profile_ids": []int{hID}},
	)

	// Kids' view: no speaker names leaked, library filtered by category.
	_, raw := e.expect(200, "GET", "/api/profiles", "", nil)
	if strings.Contains(string(raw), `"speaker"`) {
		t.Errorf("public profiles leak speaker: %s", raw)
	}

	var music, books []map[string]any

	_, raw = e.expect(200, "GET", "/api/profiles/"+itoa(hID)+"/library?category=music", "", nil)
	_ = json.Unmarshal(raw, &music)
	_, raw = e.expect(200, "GET", "/api/profiles/"+itoa(hID)+"/library?category=audiobook", "", nil)

	_ = json.Unmarshal(raw, &books)
	if len(music) != 1 || len(books) != 1 || music[0]["title"] != "Kids Rock" || books[0]["title"] != "Zugpferdli" {
		t.Fatalf("music=%v books=%v", music, books)
	}

	e.expect(400, "GET", "/api/profiles/"+itoa(hID)+"/library?category=x", "", nil)

	// Play: goes to the profile's own speaker, with the profile's volume cap.
	itemID := int(music[0]["id"].(float64))
	e.expect(200, "POST", "/api/profiles/"+itoa(hID)+"/play", "", map[string]any{"item_id": itemID})

	if got := e.player.calls[len(e.player.calls)-1]; got != "play Globi Tidal album 76466264 max=30" {
		t.Errorf("play call: %q", got)
	}
	// Globine cannot play an item from Globi's library.
	e.expect(404, "POST", "/api/profiles/"+itoa(lID)+"/play", "", map[string]any{"item_id": itemID})

	// Volume is clamped to the profile's maximum.
	res, _ = e.expect(200, "POST", "/api/profiles/"+itoa(hID)+"/volume", "", map[string]any{"level": 90})
	if res["level"].(float64) != 30 || e.player.volume != 30 {
		t.Errorf("volume not clamped: %v / %d", res, e.player.volume)
	}

	e.expect(400, "POST", "/api/profiles/"+itoa(hID)+"/volume", "", map[string]any{})

	e.expect(200, "POST", "/api/profiles/"+itoa(hID)+"/pause", "", nil)
	e.expect(200, "POST", "/api/profiles/"+itoa(lID)+"/next", "", nil)

	calls := strings.Join(e.player.calls, "|")
	if !strings.Contains(calls, "pause Globi") || !strings.Contains(calls, "next Globine") {
		t.Errorf("control calls: %s", calls)
	}

	res, _ = e.expect(200, "GET", "/api/profiles/"+itoa(hID)+"/status", "", nil)
	if res["state"] != "PLAYING" || res["max_volume"].(float64) != 30 {
		t.Errorf("status: %v", res)
	}

	// Speaker failure surfaces as 502, not a crash.
	e.player.err = errors.New("speaker offline")
	e.expect(502, "POST", "/api/profiles/"+itoa(hID)+"/pause", "", nil)

	// Deleting a profile removes its library.
	e.expect(204, "DELETE", "/api/admin/profiles/"+itoa(hID), "secret", nil)
	e.expect(404, "GET", "/api/profiles/"+itoa(hID)+"/library", "", nil)
}

func TestAddNeedsTitleWhenLookupFails(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{}) // fake lookup fails
	p, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "A", "speaker": "A"})
	id := int(p["id"].(float64))
	e.expect(
		422,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "76466264", "category": "music", "profile_ids": []int{id}},
	)
	e.expect(
		201,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "76466264", "category": "music", "profile_ids": []int{id}, "title": "Typed"},
	)
}

func TestStaticUI(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{})

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))

		return rec
	}
	if get("/").Code == 404 && strings.Contains(get("/").Body.String(), "UI not built") {
		t.Skip("ui not built")
	}

	root := get("/")
	if root.Code != 200 || !strings.Contains(root.Body.String(), `id="root"`) {
		t.Fatalf("/: %d %s", root.Code, root.Body.String())
	}
	// A client-side route falls back to index.html.
	if deep := get("/some/client/route"); deep.Code != 200 || deep.Body.String() != root.Body.String() {
		t.Errorf("spa fallback: %d", deep.Code)
	}
	// Unknown API paths are JSON 404s, never HTML.
	if api := get("/api/nope"); api.Code != 404 || !strings.Contains(api.Header().Get("Content-Type"), "json") {
		t.Errorf("api 404: %d %v", api.Code, api.Header())
	}
	// /admin is a route of the React app: same page, but marked noindex.
	if adm := get(
		"/admin",
	); adm.Code != 200 || adm.Body.String() != root.Body.String() ||
		adm.Header().Get("X-Robots-Tag") != "noindex" {
		t.Errorf("admin route: %d %v", adm.Code, adm.Header())
	}

	if get("/").Header().Get("X-Robots-Tag") != "" {
		t.Error("kids' page must not be noindex")
	}
}

func idOf(m map[string]any) int { return int(m["id"].(float64)) }

func TestCategoriesArtistsAndOrdering(t *testing.T) {
	e := newEnv(
		t,
		"secret",
		meta.Info{
			Title:       "Looked Up",
			CoverURL:    "https://img/c.jpg",
			Artist:      "Looked Up Artist",
			ReleaseDate: "2020-02-02",
		},
	)
	p, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "Globi"})
	pid := idOf(p)

	// Without a category, a playlist link lands in Playlists and everything else in Music.
	res, _ := e.expect(
		201,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{
			"url":         "https://tidal.com/playlist/96668cab-92d7-4f74-8b9b-cf54cbbe9ed3",
			"profile_ids": []int{pid},
		},
	)
	if res["items"].([]any)[0].(map[string]any)["category"] != "playlist" {
		t.Errorf("playlist category: %v", res)
	}

	res, _ = e.expect(
		201,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "76466264", "profile_ids": []int{pid}},
	)

	first := res["items"].([]any)[0].(map[string]any)
	if first["category"] != "music" || first["artist"] != "Looked Up Artist" || first["release_date"] != "2020-02-02" {
		t.Errorf("music item from lookup: %v", first)
	}

	// Typed values override the lookup; dates are normalised.
	e.expect(201, "POST", "/api/admin/items", "secret", map[string]any{
		"url": "spotify:album:aaa", "category": "audiobook", "profile_ids": []int{pid},
		"title": "Folge 3", "artist": "Conni", "release_date": "2015",
	})
	e.expect(201, "POST", "/api/admin/items", "secret", map[string]any{
		"url": "spotify:album:bbb", "category": "audiobook", "profile_ids": []int{pid},
		"title": "Folge 1", "artist": "Conni", "release_date": "2008-06",
	})
	e.expect(201, "POST", "/api/admin/items", "secret", map[string]any{
		"url": "spotify:album:ccc", "category": "audiobook", "profile_ids": []int{pid},
		"title": "Folge 2", "artist": "Conni", "release_date": "2010-03-04",
	})
	e.expect(
		400,
		"POST",
		"/api/admin/items",
		"secret",
		map[string]any{"url": "76466264", "category": "podcast", "profile_ids": []int{pid}},
	)

	// Audiobooks come back oldest first; the library lists categories in a fixed order.
	_, raw := e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/library?category=audiobook", "", nil)

	var books []map[string]any

	_ = json.Unmarshal(raw, &books)

	var titles []string
	for _, b := range books {
		titles = append(titles, b["title"].(string)+"@"+b["release_date"].(string))
	}

	if strings.Join(titles, "|") != "Folge 1@2008-06-01|Folge 2@2010-03-04|Folge 3@2015-01-01" {
		t.Errorf("audiobooks: %v", titles)
	}

	e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/library?category=playlist", "", nil)

	// Artist, date and category can be edited later.
	id := idOf(books[0])

	upd, _ := e.expect(
		200,
		"PUT",
		"/api/admin/items/"+itoa(id),
		"secret",
		map[string]any{"artist": "Conni Pony", "release_date": "", "category": "music"},
	)
	if upd["artist"] != "Conni Pony" || upd["release_date"] != "" || upd["category"] != "music" {
		t.Errorf("update: %v", upd)
	}

	e.expect(400, "PUT", "/api/admin/items/"+itoa(id), "secret", map[string]any{"category": "nope"})
}

func TestDiscographyAndBatch(t *testing.T) {
	cat := fakeCatalog{d: catalog.Discography{
		Service: "spotify",
		Artist:  catalog.Artist{Name: "Conni", ImageURL: "https://img/conni.jpg"},
		Albums: []catalog.Album{
			{
				ID:          "a1",
				Title:       "Folge 1",
				Artist:      "Conni",
				CoverURL:    "https://img/a1.jpg",
				ReleaseDate: "2008-01-01",
				Type:        "album",
			},
			{ID: "a2", Title: "Folge 2", Artist: "Conni", ReleaseDate: "2010-03-04", Type: "single"},
		},
	}}
	e := newEnvWith(t, "secret", meta.Info{Title: "Looked Up", CoverURL: "https://img/looked-up.jpg"}, cat)
	h, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "Globi"})
	l, _ := e.expect(
		201,
		"POST",
		"/api/admin/profiles",
		"secret",
		map[string]any{"name": "Globine", "speaker": "Globine"},
	)
	hid, lid := idOf(h), idOf(l)

	d, _ := e.expect(
		200,
		"POST",
		"/api/admin/discography",
		"secret",
		map[string]any{"url": "https://open.spotify.com/artist/ART/discography/all"},
	)

	albums := d["albums"].([]any)
	if d["service"] != "spotify" || d["artist"].(map[string]any)["name"] != "Conni" || len(albums) != 2 ||
		albums[0].(map[string]any)["title"] != "Folge 1" {
		t.Errorf("discography: %v", d)
	}

	e.expect(401, "POST", "/api/admin/discography", "", map[string]any{"url": "x"})

	batch := func(profiles []int, items []map[string]any) (int, map[string]any) {
		code, m, raw := e.do(
			"POST",
			"/api/admin/items/batch",
			"secret",
			map[string]any{"category": "audiobook", "profile_ids": profiles, "items": items},
		)
		if code != 201 && code != 409 {
			t.Fatalf("batch: %d %s", code, raw)
		}

		return code, m
	}
	items := []map[string]any{
		{
			"service":      "spotify",
			"kind":         "album",
			"id":           "a2",
			"title":        "Folge 2",
			"artist":       "Conni",
			"release_date": "2010-03-04",
		}, // no cover: looked up
		{
			"service":      "spotify",
			"kind":         "album",
			"id":           "a1",
			"title":        "Folge 1",
			"artist":       "Conni",
			"release_date": "2008",
			"cover_url":    "https://img/a1.jpg",
		},
	}

	code, res := batch([]int{hid, lid}, items)
	if code != 201 || res["created"].(float64) != 4 || res["duplicates"].(float64) != 0 {
		t.Errorf("first batch: %d %v", code, res)
	}
	// Running it again only reports duplicates and answers 409.
	code, res = batch([]int{hid}, items)
	if code != 409 || res["created"].(float64) != 0 || res["duplicates"].(float64) != 2 {
		t.Errorf("repeat batch: %d %v", code, res)
	}

	_, raw := e.expect(200, "GET", "/api/profiles/"+itoa(lid)+"/library?category=audiobook", "", nil)

	var lib []map[string]any

	_ = json.Unmarshal(raw, &lib)
	if len(lib) != 2 || lib[0]["title"] != "Folge 1" || lib[0]["release_date"] != "2008-01-01" ||
		lib[1]["cover_url"] != "https://img/looked-up.jpg" {
		t.Errorf("library after batch: %v", lib)
	}

	// Bad input is rejected as a whole before anything is written.
	for name, body := range map[string]map[string]any{
		"unknown service": {"category": "music", "profile_ids": []int{hid}, "items": []map[string]any{{"service": "deezer", "kind": "album", "id": "1", "title": "x"}}},
		"bad id":          {"category": "music", "profile_ids": []int{hid}, "items": []map[string]any{{"service": "tidal", "kind": "album", "id": "../x", "title": "x"}}},
		"no title":        {"category": "music", "profile_ids": []int{hid}, "items": []map[string]any{{"service": "tidal", "kind": "album", "id": "1", "title": " "}}},
		"bad category":    {"category": "x", "profile_ids": []int{hid}, "items": []map[string]any{{"service": "tidal", "kind": "album", "id": "1", "title": "t"}}},
		"no items":        {"category": "music", "profile_ids": []int{hid}, "items": []map[string]any{}},
		"no profiles":     {"category": "music", "profile_ids": []int{}, "items": []map[string]any{{"service": "tidal", "kind": "album", "id": "1", "title": "t"}}},
	} {
		if code, _, raw := e.do("POST", "/api/admin/items/batch", "secret", body); code != 400 {
			t.Errorf("%s: got %d %s", name, code, raw)
		}
	}
}

func TestDiscographyErrors(t *testing.T) {
	cases := []struct {
		name string
		cat  Catalog
		want int
		code string
	}{
		{"no catalog at all", nil, 503, "not_configured"},
		{
			"service not configured",
			fakeCatalog{err: catalog.NotConfiguredError{Service: "Tidal"}},
			503,
			"not_configured",
		},
		{"not an artist link", fakeCatalog{err: catalog.ErrBadLink}, 400, ""},
		{"upstream failure", fakeCatalog{err: errors.New("spotify exploded")}, 502, ""},
	}
	for _, c := range cases {
		e := newEnvWith(t, "secret", meta.Info{}, c.cat)

		m, _ := e.expect(
			c.want,
			"POST",
			"/api/admin/discography",
			"secret",
			map[string]any{"url": "https://tidal.com/artist/1"},
		)
		if code, _ := m["code"].(string); code != c.code {
			t.Errorf("%s: code %q, want %q (%v)", c.name, code, c.code, m)
		}
		// The UI names the missing service, so the response must say which.
		if c.code == "not_configured" && m["service"] != "Tidal" {
			t.Errorf("%s: service %v, want Tidal", c.name, m["service"])
		}
	}
}

// blockingMeta never answers until its context ends, like a hung service.
type blockingMeta struct{}

func (blockingMeta) Fetch(ctx context.Context, _ sonos.Ref) (meta.Info, error) {
	<-ctx.Done()
	return meta.Info{}, ctx.Err()
}

func TestBatchDoesNotWaitForSlowCoverLookups(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	p, _ := st.CreateProfile(context.Background(), store.Profile{Name: "A", Speaker: "A"})
	h := New(
		Options{
			Store:        st,
			Player:       &fakePlayer{},
			Meta:         blockingMeta{},
			AdminToken:   "secret",
			CoverTimeout: 50 * time.Millisecond,
		},
	).Handler()

	var items []map[string]any
	for i := range 9 { // more than the 4 lookups that run at once
		items = append(
			items,
			map[string]any{
				"service": "spotify",
				"kind":    "album",
				"id":      fmt.Sprintf("id%d", i),
				"title":   fmt.Sprintf("T%d", i),
			},
		)
	}

	body, err := json.Marshal(map[string]any{"category": "music", "profile_ids": []int64{p.ID}, "items": items})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/api/admin/items/batch",
		bytes.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer secret")

	rec := httptest.NewRecorder()

	start := time.Now()

	h.ServeHTTP(rec, req)

	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("import blocked on cover lookups for %v", took)
	}

	if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"created":9`) {
		t.Errorf("batch: %d %s", rec.Code, rec.Body.String())
	}

	if got, _ := st.ListItems(context.Background(), p.ID, ""); len(got) != 9 || got[0].CoverURL != "" {
		t.Errorf("items saved without covers: %d %+v", len(got), got[0])
	}
}

func TestDiscographyWithoutCatalogRejectsNonArtistLinks(t *testing.T) {
	e := newEnvWith(t, "secret", meta.Info{}, nil)
	e.expect(400, "POST", "/api/admin/discography", "secret", map[string]any{"url": "https://tidal.com/album/1"})
}

// mapMeta answers per release, like the real services would.
type mapMeta map[string]meta.Info

func (m mapMeta) Fetch(_ context.Context, ref sonos.Ref) (meta.Info, error) {
	if info, ok := m[ref.ID]; ok {
		return info, nil
	}

	return meta.Info{}, errors.New("not found")
}

func TestBackfillMetadata(t *testing.T) {
	ctx := context.Background()

	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	h, _ := st.CreateProfile(ctx, store.Profile{Name: "Globi", Speaker: "Globi"})
	l, _ := st.CreateProfile(ctx, store.Profile{Name: "Globine", Speaker: "Globine"})
	add := func(p store.Profile, it store.Item) store.Item {
		t.Helper()

		it.ProfileID, it.Category = p.ID, cmpCategory(it)

		got, err := st.AddItem(ctx, it)
		if err != nil {
			t.Fatal(err)
		}

		return got
	}
	// As saved by the first version: no artist or date, Tidal title as "Artist - Title".
	tidal := add(h, store.Item{Title: "Jack and the Weatherman - Ruins", Service: "tidal", Kind: "album", RefID: "T1"})
	spotify := add(
		h,
		store.Item{Title: "Glück auf den Strassen - Deluxe Edition", Service: "spotify", Kind: "album", RefID: "S1"},
	)
	shared := add(
		l,
		store.Item{Title: "Glück auf den Strassen - Deluxe Edition", Service: "spotify", Kind: "album", RefID: "S1"},
	) // same release, other kid
	playlist := add(h, store.Item{Title: "Einschlafmusik", Service: "spotify", Kind: "playlist", RefID: "P1"})
	byHand := add(
		h,
		store.Item{Title: "Mine", Artist: "Typed By Hand", Service: "tidal", Kind: "album", RefID: "T2"},
	) // has an artist, lacks a date
	unknown := add(h, store.Item{Title: "No Info Anywhere", Service: "tidal", Kind: "album", RefID: "T3"})

	var lookups atomic.Int32

	srv := New(Options{Store: st, Player: &fakePlayer{}, AdminToken: "secret", Meta: countingMeta{
		&lookups,
		mapMeta{
			"T1": {
				Title:       "Ruins",
				Artist:      "Jack and the Weatherman",
				ReleaseDate: "2016-05-06",
				CoverURL:    "https://img/t1.jpg",
			},
			"S1": {
				Title:       "Glück auf den Strassen - Deluxe Edition",
				Artist:      "Dabu Fantastic",
				ReleaseDate: "2019-02-01",
			},
			"T2": {Title: "Mine", Artist: "Someone Else", ReleaseDate: "2011-01-01"},
		},
	}})

	res, err := srv.BackfillMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if res.Checked != 4 || lookups.Load() != 4 { // T1, S1 (once for both kids), T2, T3; the playlist is not looked up
		t.Errorf(
			"checked %d releases with %d lookups, want 4 (one per release, none for playlists)",
			res.Checked,
			lookups.Load(),
		)
	}

	if res.Updated != 4 { // T1, S1 for Globi, S1 for Globine, T2's date
		t.Errorf("updated %d entries, want 4", res.Updated)
	}

	get := func(it store.Item) store.Item {
		t.Helper()

		got, err := st.GetItem(ctx, it.ID)
		if err != nil {
			t.Fatal(err)
		}

		return got
	}
	if g := get(
		tidal,
	); g.Artist != "Jack and the Weatherman" || g.Title != "Ruins" || g.ReleaseDate != "2016-05-06" ||
		g.CoverURL != "https://img/t1.jpg" {
		t.Errorf("tidal item: %+v", g)
	}

	for _, it := range []store.Item{spotify, shared} {
		if g := get(
			it,
		); g.Artist != "Dabu Fantastic" || g.ReleaseDate != "2019-02-01" ||
			g.Title != "Glück auf den Strassen - Deluxe Edition" {
			t.Errorf(
				"spotify item for profile %d: %+v",
				it.ProfileID,
				g,
			) // the title has no artist prefix and must stay as is
		}
	}

	if g := get(byHand); g.Artist != "Typed By Hand" || g.ReleaseDate != "2011-01-01" {
		t.Errorf("a hand-typed artist must be kept and only the date filled: %+v", g)
	}

	if g := get(playlist); g.Artist != "" || g.ReleaseDate != "" {
		t.Errorf("playlist changed: %+v", g)
	}

	if g := get(unknown); g.Artist != "" || g.Title != "No Info Anywhere" {
		t.Errorf("an item the service knows nothing about must be left alone: %+v", g)
	}

	// Nothing more to learn: a second run changes nothing.
	if res, _ = srv.BackfillMetadata(ctx); res.Updated != 0 {
		t.Errorf("second run updated %d", res.Updated)
	}

	// The same thing is available to the admin on demand.
	h2 := srv.Handler()

	for _, tc := range []struct {
		token string
		want  int
	}{{"", 401}, {"secret", 200}} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/admin/items/refresh", nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}

		rec := httptest.NewRecorder()
		h2.ServeHTTP(rec, req)

		if rec.Code != tc.want {
			t.Errorf("refresh with token %q: %d %s", tc.token, rec.Code, rec.Body.String())
		}
	}
}

func cmpCategory(it store.Item) string {
	if it.Kind == "playlist" {
		return store.CategoryPlaylist
	}

	return store.CategoryMusic
}

// countingMeta counts how many lookups were made.
type countingMeta struct {
	n    *atomic.Int32
	next MetaFetcher
}

func (c countingMeta) Fetch(ctx context.Context, ref sonos.Ref) (meta.Info, error) {
	c.n.Add(1)
	return c.next.Fetch(ctx, ref)
}

func TestPlayedFlagIsPerKidAndOnlySetWhenPlaybackStarted(t *testing.T) {
	e := newEnv(t, "secret", meta.Info{Title: "Folge 1", Artist: "Conni", ReleaseDate: "2008-03-01"})
	h, _ := e.expect(201, "POST", "/api/admin/profiles", "secret", map[string]any{"name": "Globi", "speaker": "Globi"})
	l, _ := e.expect(
		201,
		"POST",
		"/api/admin/profiles",
		"secret",
		map[string]any{"name": "Globine", "speaker": "Globine"},
	)
	hid, lid := idOf(h), idOf(l)
	// The same two episodes for both kids.
	for _, ref := range []string{"spotify:album:aaa", "spotify:album:bbb"} {
		e.expect(
			201,
			"POST",
			"/api/admin/items",
			"secret",
			map[string]any{"url": ref, "category": "audiobook", "profile_ids": []int{hid, lid}},
		)
	}

	library := func(pid int) []map[string]any {
		t.Helper()

		_, raw := e.expect(200, "GET", "/api/profiles/"+itoa(pid)+"/library?category=audiobook", "", nil)

		var items []map[string]any

		_ = json.Unmarshal(raw, &items)

		return items
	}
	for _, it := range library(hid) {
		if it["played"] != false || it["played_at"] != "" {
			t.Fatalf("new items start unplayed: %v", it)
		}
	}

	first := library(hid)[0]
	// A failed start must not count as played.
	e.player.err = errors.New("speaker offline")
	e.expect(502, "POST", "/api/profiles/"+itoa(hid)+"/play", "", map[string]any{"item_id": idOf(first)})

	if library(hid)[0]["played"] != false {
		t.Error("an item whose playback failed was marked as played")
	}

	e.player.err = nil

	// A successful start marks it, for that kid only.
	e.expect(200, "POST", "/api/profiles/"+itoa(hid)+"/play", "", map[string]any{"item_id": idOf(first)})

	got := library(hid)
	if got[0]["played"] != true || got[0]["played_at"] == "" || got[1]["played"] != false {
		t.Errorf("globi after playing the first episode: %v", got)
	}

	for _, it := range library(lid) {
		if it["played"] != false {
			t.Errorf("globine's copy must stay unplayed: %v", it)
		}
	}

	// The admin can reset it and mark it again; other edits keep it.
	reset, _ := e.expect(200, "PUT", "/api/admin/items/"+itoa(idOf(first)), "secret", map[string]any{"played": false})
	if reset["played"] != false {
		t.Errorf("reset: %v", reset)
	}

	marked, _ := e.expect(200, "PUT", "/api/admin/items/"+itoa(idOf(first)), "secret", map[string]any{"played": true})

	renamed, _ := e.expect(
		200,
		"PUT",
		"/api/admin/items/"+itoa(idOf(first)),
		"secret",
		map[string]any{"title": "Renamed"},
	)
	if marked["played"] != true || renamed["played"] != true || renamed["title"] != "Renamed" {
		t.Errorf("marked=%v renamed=%v", marked, renamed)
	}
}
