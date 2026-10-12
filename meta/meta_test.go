package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

func TestFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oembed", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://open.spotify.com/album/abc" {
			http.Error(w, "bad url", http.StatusBadRequest)
			return
		}

		_, _ = w.Write([]byte(`{"title":"Kids Rock","thumbnail_url":"https://img/x.jpg"}`))
	})
	mux.HandleFunc("/album/abc", func(w http.ResponseWriter, r *http.Request) { // Spotify page
		_, _ = w.Write(
			[]byte(
				`<meta property="og:description" content="Various Artists &amp; Co · compilation · 2022 · 100 songs"/><meta name="music:release_date" content="2022-09-30"/>`,
			),
		)
	})
	mux.HandleFunc("/playlist/pl", func(w http.ResponseWriter, r *http.Request) {
		t.Error("playlist pages must not be fetched for an artist")
	})
	mux.HandleFunc("/album/76466264", func(w http.ResponseWriter, r *http.Request) { // Tidal page
		_, _ = w.Write(
			[]byte(
				`<head><meta property="og:title" content="Bluey &amp; Friends - Theme - Live"><meta property="og:image" content="https://img/t.jpg"><meta property="music:release_date" content="2017-07-28"></head>`,
			),
		)
	})
	mux.HandleFunc("/playlist/76466264", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<meta property="og:title" content="Kids - Hits">`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := &Fetcher{Client: srv.Client(), SpotifyOEmbed: srv.URL + "/oembed", SpotifyBase: srv.URL, TidalBase: srv.URL}
	ctx := context.Background()

	got, err := f.Fetch(ctx, sonos.Ref{Service: sonos.Spotify, Kind: sonos.Album, ID: "abc"})
	if err != nil ||
		got != (Info{Title: "Kids Rock", CoverURL: "https://img/x.jpg", Artist: "Various Artists & Co", ReleaseDate: "2022-09-30"}) {
		t.Errorf("spotify: %+v %v", got, err)
	}

	got, err = f.Fetch(ctx, sonos.Ref{Service: sonos.Tidal, Kind: sonos.Album, ID: "76466264"})
	// The title splits at the first " - ": artist on the left, album on the right.
	if err != nil ||
		got != (Info{Title: "Theme - Live", CoverURL: "https://img/t.jpg", Artist: "Bluey & Friends", ReleaseDate: "2017-07-28"}) {
		t.Errorf("tidal: %+v %v", got, err)
	}
	// Playlist titles are never split into artist and title.
	got, err = f.Fetch(ctx, sonos.Ref{Service: sonos.Tidal, Kind: sonos.Playlist, ID: "76466264"})
	if err != nil || got.Title != "Kids - Hits" || got.Artist != "" {
		t.Errorf("tidal playlist: %+v %v", got, err)
	}

	if _, err := f.Fetch(ctx, sonos.Ref{Service: sonos.Tidal, Kind: sonos.Album, ID: "missing"}); err == nil {
		t.Error("expected error for missing page")
	}
}

func TestNormalizeDate(t *testing.T) {
	for in, want := range map[string]string{
		"2022": "2022-01-01", "2022-09": "2022-09-01", "2022-09-30": "2022-09-30",
		"2022-09-30T10:00:00Z": "2022-09-30", " 1999 ": "1999-01-01",
		"": "", "soon": "", "2022-13-40": "", "22": "",
	} {
		if got := NormalizeDate(in); got != want {
			t.Errorf("NormalizeDate(%q) = %q, want %q", in, got, want)
		}
	}
}
