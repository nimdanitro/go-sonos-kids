package catalog

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseArtistLink(t *testing.T) {
	cases := []struct {
		in, svc, id string
		err         bool
	}{
		{"https://open.spotify.com/artist/0jRC2uXx4hEpqNrBDJdY7l/discography/all", "spotify", "0jRC2uXx4hEpqNrBDJdY7l", false},
		{"https://open.spotify.com/intl-de/artist/0jRC2uXx4hEpqNrBDJdY7l?si=abc", "spotify", "0jRC2uXx4hEpqNrBDJdY7l", false},
		{"spotify:artist:0jRC2uXx4hEpqNrBDJdY7l", "spotify", "0jRC2uXx4hEpqNrBDJdY7l", false},
		{"https://tidal.com/artist/55839", "tidal", "55839", false},
		{"https://tidal.com/browse/artist/55839/", "tidal", "55839", false},
		{"https://open.spotify.com/album/6TDpQggB88X230Srv1fkSq", "", "", true},
		{"https://tidal.com/album/76466264", "", "", true},
		{"nonsense", "", "", true},
	}
	for _, c := range cases {
		svc, id, err := ParseArtistLink(c.in)
		if (err != nil) != c.err || svc != c.svc || id != c.id {
			t.Errorf("%q: got %q %q %v", c.in, svc, id, err)
		}
	}
}

func TestSpotifyDiscography(t *testing.T) {
	var tokenCalls atomic.Int32
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("id:secret"))
		if r.Header.Get("Authorization") != want || r.FormValue("grant_type") != "client_credentials" {
			http.Error(w, "bad credentials", 401)
			return
		}
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, "no token", 401)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/v1/artists/ART", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Conni","images":[{"url":"https://img/big.jpg"},{"url":"https://img/small.jpg"}]}`)
	}))
	mux.HandleFunc("/v1/artists/ART/albums", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" { // real "next" links repeat the original query; the fake keeps it short
			fmt.Fprint(w, `{"items":[{"id":"a1","name":"Conni kommt in die Schule","album_type":"album","release_date":"2008","images":[{"url":"https://img/a1.jpg"}]}],"next":null}`)
			return
		}
		// The first request must ask for every release group, limited to the market.
		if r.URL.Query().Get("market") != "CH" || !strings.Contains(r.URL.Query().Get("include_groups"), "single") {
			t.Errorf("first request query: %v", r.URL.RawQuery)
		}
		fmt.Fprintf(w, `{"items":[
			{"id":"a3","name":"Conni geht zelten","album_type":"album","release_date":"2015-06","images":[{"url":"https://img/a3.jpg"}]},
			{"id":"a2","name":"Conni Single","album_type":"single","release_date":"2010-03-04","images":[]}],
			"next":"%s/v1/artists/ART/albums?page=2"}`, srv.URL)
	}))
	srv = httptest.NewServer(mux)
	defer srv.Close()

	p := NewSpotify("id", "secret", "CH")
	p.APIBase = srv.URL + "/v1"
	p.tokens.url = srv.URL + "/token"

	svc := Service{Providers: map[string]Provider{"spotify": p}}
	d, err := svc.Discography(context.Background(), "https://open.spotify.com/artist/ART/discography/all")
	if err != nil {
		t.Fatal(err)
	}
	if d.Service != "spotify" || d.Artist.Name != "Conni" || d.Artist.ImageURL != "https://img/big.jpg" {
		t.Errorf("artist: %+v", d)
	}
	// Both pages are read, dates normalised, and the result is oldest first.
	var got []string
	for _, a := range d.Albums {
		got = append(got, a.ReleaseDate+" "+a.Title+" ["+a.Type+"] by "+a.Artist)
	}
	want := []string{
		"2008-01-01 Conni kommt in die Schule [album] by Conni",
		"2010-03-04 Conni Single [single] by Conni",
		"2015-06-01 Conni geht zelten [album] by Conni",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("albums\n got %v\nwant %v", got, want)
	}
	if d.Albums[0].CoverURL != "https://img/a1.jpg" || d.Albums[1].CoverURL != "" {
		t.Errorf("covers: %q %q", d.Albums[0].CoverURL, d.Albums[1].CoverURL)
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Errorf("token requested %d times, want 1 (cached)", n)
	}

	// An expired/revoked token is refreshed once and the call retried.
	p.tokens.mu.Lock()
	p.tokens.token = "stale"
	p.tokens.mu.Unlock()
	if _, err := svc.Discography(context.Background(), "spotify:artist:ART"); err != nil {
		t.Errorf("after stale token: %v", err)
	}
	if n := tokenCalls.Load(); n != 2 {
		t.Errorf("token requested %d times after refresh, want 2", n)
	}

	// Wrong credentials give a helpful error.
	bad := NewSpotify("id", "wrong", "CH")
	bad.APIBase, bad.tokens.url = srv.URL+"/v1", srv.URL+"/token"
	if _, err := bad.Discography(context.Background(), "ART"); err == nil || !strings.Contains(err.Error(), "client ID and secret") {
		t.Errorf("bad credentials: %v", err)
	}
}

func TestTidalDiscography(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})
	mux.HandleFunc("/v2/artists/55839", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || !strings.Contains(r.Header.Get("Accept"), "vnd.api+json") {
			t.Errorf("headers: %v", r.Header)
		}
		fmt.Fprint(w, `{"data":{"id":"55839","type":"artists","attributes":{"name":"Manchester Orchestra"}}}`)
	})
	mux.HandleFunc("/v2/artists/55839/relationships/albums", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("countryCode") != "CH" {
			t.Errorf("country: %v", r.URL.RawQuery)
		}
		if r.URL.Query().Get("page[cursor]") == "c2" {
			fmt.Fprint(w, `{"data":[{"id":"3","type":"albums"}],"included":[
				{"id":"3","type":"albums","attributes":{"title":"Older Album","releaseDate":"2011-05-03","albumType":"SINGLE"}}],"links":{}}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"1","type":"albums"},{"id":"2","type":"albums"}],"included":[
			{"id":"1","type":"albums","attributes":{"title":"A Black Mile","releaseDate":"2017-07-28","albumType":"ALBUM"},"relationships":{"coverArt":{"data":[{"id":"art1","type":"artworks"}]}}},
			{"id":"2","type":"albums","attributes":{"title":"Some EP","releaseDate":"2019-01-01","albumType":"EP"}},
			{"id":"art1","type":"artworks","attributes":{"files":[{"href":"https://img/small.jpg","meta":{"width":80}},{"href":"https://img/big.jpg","meta":{"width":1280}}]}}],
			"links":{"next":"/artists/55839/relationships/albums?countryCode=CH&include=albums&page%5Bcursor%5D=c2"}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewTidal("id", "secret", "CH")
	p.APIBase = srv.URL + "/v2"
	p.tokens.url = srv.URL + "/token"

	d, err := Service{Providers: map[string]Provider{"tidal": p}}.Discography(context.Background(), "https://tidal.com/artist/55839")
	if err != nil {
		t.Fatal(err)
	}
	if d.Artist.Name != "Manchester Orchestra" || len(d.Albums) != 3 {
		t.Fatalf("discography: %+v", d)
	}
	titles := []string{d.Albums[0].Title, d.Albums[1].Title, d.Albums[2].Title}
	if strings.Join(titles, "|") != "Older Album|A Black Mile|Some EP" {
		t.Errorf("order: %v", titles)
	}
	if d.Albums[0].Type != "single" || d.Albums[2].Type != "album" {
		t.Errorf("types: %q %q", d.Albums[0].Type, d.Albums[2].Type)
	}
	if d.Albums[1].CoverURL != "https://img/big.jpg" || d.Albums[1].Artist != "Manchester Orchestra" {
		t.Errorf("album: %+v", d.Albums[1])
	}
}

func TestNotConfiguredAndBadLink(t *testing.T) {
	svc := Service{Providers: map[string]Provider{}}
	_, err := svc.Discography(context.Background(), "https://tidal.com/artist/1")
	var nc NotConfiguredError
	if !errors.As(err, &nc) || nc.Service != "Tidal" {
		t.Errorf("not configured: %v", err)
	}
	if _, err := svc.Discography(context.Background(), "https://example.com"); !errors.Is(err, ErrBadLink) {
		t.Errorf("bad link: %v", err)
	}
}

// fakeSpotify serves 25 albums but, like Spotify for newer apps, answers any
// page size above maxLimit with a 400.
func fakeSpotify(t *testing.T, maxLimit int, firstRequestLimits *[]string) *Spotify {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"access_token":"tok","expires_in":3600}`)
	})
	mux.HandleFunc("/v1/artists/ART", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Conni","images":[]}`)
	})
	mux.HandleFunc("/v1/artists/ART/albums", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 50
		fmt.Sscan(q.Get("limit"), &limit)
		offset := 0
		fmt.Sscan(q.Get("offset"), &offset)
		if offset == 0 {
			*firstRequestLimits = append(*firstRequestLimits, q.Get("limit"))
		}
		if limit > maxLimit {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"status":400,"message":"Invalid limit"}}`)
			return
		}
		var items []string
		for i := offset; i < offset+limit && i < 25; i++ {
			items = append(items, fmt.Sprintf(`{"id":"a%d","name":"Folge %d","album_type":"album","release_date":"%d","images":[]}`, i, i, 2000+i))
		}
		next := "null"
		if offset+limit < 25 {
			// Real "next" links repeat the original query, including the limit.
			next = fmt.Sprintf(`"%s/v1/artists/ART/albums?limit=%d&offset=%d&market=CH"`, srv.URL, limit, offset+limit)
		}
		fmt.Fprintf(w, `{"items":[%s],"next":%s}`, strings.Join(items, ","), next)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := NewSpotify("id", "secret", "CH")
	p.APIBase, p.tokens.url = srv.URL+"/v1", srv.URL+"/token"
	return p
}

func TestSpotifyFallsBackToSmallPages(t *testing.T) {
	var firstLimits []string
	p := fakeSpotify(t, 10, &firstLimits)

	d, err := p.Discography(context.Background(), "ART")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Albums) != 25 || d.Albums[0].Title != "Folge 0" || d.Albums[24].Title != "Folge 24" {
		t.Fatalf("got %d albums, first %q, last %q", len(d.Albums), d.Albums[0].Title, d.Albums[len(d.Albums)-1].Title)
	}
	// It asked for 50 once, was refused, and started over at 10.
	if strings.Join(firstLimits, ",") != "50,10" {
		t.Errorf("first-page limits: %v", firstLimits)
	}

	// The smaller size is remembered, so the next import does not retry 50.
	firstLimits = nil
	if d, err = p.Discography(context.Background(), "ART"); err != nil || len(d.Albums) != 25 {
		t.Fatalf("second import: %d albums, %v", len(d.Albums), err)
	}
	if strings.Join(firstLimits, ",") != "10" {
		t.Errorf("second import first-page limits: %v", firstLimits)
	}
}

func TestSpotifyKeepsLargePagesWhenAllowed(t *testing.T) {
	var firstLimits []string
	p := fakeSpotify(t, 50, &firstLimits)
	d, err := p.Discography(context.Background(), "ART")
	if err != nil || len(d.Albums) != 25 {
		t.Fatalf("%d albums, %v", len(d.Albums), err)
	}
	if strings.Join(firstLimits, ",") != "50" {
		t.Errorf("limits: %v (must not drop to 10 when 50 works)", firstLimits)
	}
}

func TestSpotifyOtherBadRequestIsReported(t *testing.T) {
	var firstLimits []string
	p := fakeSpotify(t, 0, &firstLimits) // refuses every size, even 10
	_, err := p.Discography(context.Background(), "ART")
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "Invalid limit") {
		t.Errorf("error should carry Spotify's own message, got: %v", err)
	}
	if strings.Join(firstLimits, ",") != "50,10" {
		t.Errorf("must retry exactly once, got limits %v", firstLimits)
	}
}
