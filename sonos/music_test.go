package sonos

import (
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		in   string
		svc  Service
		kind Kind
		id   string
		err  bool
	}{
		{"https://tidal.com/playlist/96668cab-92d7-4f74-8b9b-cf54cbbe9ed3", Tidal, Playlist, "96668cab-92d7-4f74-8b9b-cf54cbbe9ed3", false},
		{"https://tidal.com/browse/album/76466264?u", Tidal, Album, "76466264", false},
		{"https://tidal.com/track/123", Tidal, Track, "123", false},
		{"76466264", Tidal, Album, "76466264", false},
		{"96668cab-92d7-4f74-8b9b-cf54cbbe9ed3", Tidal, Playlist, "96668cab-92d7-4f74-8b9b-cf54cbbe9ed3", false},
		{"https://open.spotify.com/playlist/5AOWnfJm4QTcTuhwcJzoKw?si=abc", Spotify, Playlist, "5AOWnfJm4QTcTuhwcJzoKw", false},
		{"https://open.spotify.com/intl-de/album/6TDpQggB88X230Srv1fkSq", Spotify, Album, "6TDpQggB88X230Srv1fkSq", false},
		{"spotify:track:6NmXV4o6bmp704aPGyTVVG", Spotify, Track, "6NmXV4o6bmp704aPGyTVVG", false},
		{"https://example.com/x", Service{}, "", "", true},
		{"hello", Service{}, "", "", true},
	}
	for _, c := range cases {
		r, err := ParseRef(c.in)
		if (err != nil) != c.err || r.Service != c.svc || r.Kind != c.kind || r.ID != c.id {
			t.Errorf("%q: got %+v %v", c.in, r, err)
		}
	}
}

func TestAccountFromFavorites(t *testing.T) {
	favs := []Favorite{
		{URI: "x-sonosapi-stream:tunein?sid=254"},
		{URI: "x-rincon-cpcontainer:1006206cspotify%3Aplaylist%3Aabc?sid=9&flags=8300&sn=228",
			Metadata: `<desc>SA_RINCON2311_X_#Svc2311-2e4e19ea-Token</desc>`},
		{URI: "x-sonos-http:track%2F1.flac?sid=174&flags=8232&sn=2",
			Metadata: `<desc>SA_RINCON44551_X_#Svc44551-0-Token</desc>`},
	}
	sp, err := accountFromFavorites(Spotify, favs)
	if err != nil || sp.SN != "228" || sp.Token != "2e4e19ea" {
		t.Errorf("spotify: %+v %v", sp, err)
	}
	td, err := accountFromFavorites(Tidal, favs)
	if err != nil || td.SN != "2" || td.Token != "0" {
		t.Errorf("tidal: %+v %v", td, err)
	}
	if _, err := accountFromFavorites(Service{Name: "X", SID: "99", Rincon: "1"}, favs); err == nil {
		t.Error("expected error for missing service")
	}
}

func TestQueueItemSpotifyAlbumMatchesFavorite(t *testing.T) {
	acc := Account{Service: Spotify, SN: "228", Token: "2e4e19ea"}
	it, err := acc.queueItem(Ref{Spotify, Album, "6TDpQggB88X230Srv1fkSq"})
	if err != nil {
		t.Fatal(err)
	}
	want := "x-rincon-cpcontainer:1004006cspotify%3Aalbum%3A6TDpQggB88X230Srv1fkSq?sid=9&flags=108&sn=228"
	if it.uri != want {
		t.Errorf("uri\n got %s\nwant %s", it.uri, want)
	}
	if !strings.Contains(it.metas[0], "SA_RINCON2311_X_#Svc2311-2e4e19ea-Token") {
		t.Error("token missing from metadata")
	}
}
