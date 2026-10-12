// Package meta looks up the title and cover art for a Tidal or Spotify link,
// so the admin only has to paste a URL.
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

// Info is what we show in the UI for a piece of content.
type Info struct {
	Title       string
	CoverURL    string
	Artist      string // empty for playlists and when unknown
	ReleaseDate string // YYYY-MM-DD, or empty when unknown
}

// NormalizeDate turns "2022", "2022-09" or "2022-09-30" into YYYY-MM-DD so that
// dates sort correctly as plain strings. Anything else yields "".
func NormalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 10 {
		s = s[:10] // e.g. an ISO timestamp
	}

	switch len(s) {
	case 4:
		s += "-01-01"
	case 7:
		s += "-01"
	}

	if _, err := time.Parse("2006-01-02", s); err != nil {
		return ""
	}

	return s
}

// Fetcher resolves Info over the public web. The base URLs are fields so tests
// can point them at a local server.
type Fetcher struct {
	Client        *http.Client
	SpotifyOEmbed string // https://open.spotify.com/oembed
	SpotifyBase   string // https://open.spotify.com
	TidalBase     string // https://tidal.com
}

func NewFetcher() *Fetcher {
	return &Fetcher{
		Client:        &http.Client{Timeout: 6 * time.Second},
		SpotifyOEmbed: "https://open.spotify.com/oembed",
		SpotifyBase:   "https://open.spotify.com",
		TidalBase:     "https://tidal.com",
	}
}

// Fetch returns the title and cover for ref. Best effort: callers should
// allow the admin to type a title when this fails.
func (f *Fetcher) Fetch(ctx context.Context, ref sonos.Ref) (Info, error) {
	switch ref.Service {
	case sonos.Spotify:
		return f.spotify(ctx, ref)
	case sonos.Tidal:
		return f.tidal(ctx, ref)
	}

	return Info{}, fmt.Errorf("no metadata source for %s", ref.Service.Name)
}

func (f *Fetcher) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; sonoskids)")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func (f *Fetcher) spotify(ctx context.Context, ref sonos.Ref) (Info, error) {
	target := fmt.Sprintf("https://open.spotify.com/%s/%s", ref.Kind, ref.ID)

	body, err := f.get(ctx, f.SpotifyOEmbed+"?url="+url.QueryEscape(target))
	if err != nil {
		return Info{}, err
	}

	var o struct {
		Title        string `json:"title"`
		ThumbnailURL string `json:"thumbnail_url"`
	}
	if err := json.Unmarshal(body, &o); err != nil {
		return Info{}, err
	}

	info := Info{Title: o.Title, CoverURL: o.ThumbnailURL}

	// oEmbed has no artist or date; the public page does. Best effort.
	if ref.Kind != sonos.Playlist {
		if page, err := f.get(ctx, fmt.Sprintf("%s/%s/%s", f.SpotifyBase, ref.Kind, ref.ID)); err == nil {
			info.ReleaseDate = NormalizeDate(metaContent(page, "music:release_date"))
			// og:description reads "Artist · album · 2022 · 100 songs".
			if desc := metaContent(page, "og:description"); desc != "" {
				if artist, _, ok := strings.Cut(desc, " · "); ok {
					info.Artist = html.UnescapeString(strings.TrimSpace(artist))
				}
			}
		}
	}

	return info, nil
}

// metaContent returns the content of a <meta property|name="key" content="..."> tag.
func metaContent(page []byte, key string) string {
	re := regexp.MustCompile(`<meta[^>]+(?:property|name)="` + regexp.QuoteMeta(key) + `"[^>]+content="([^"]*)"`)
	if m := re.FindSubmatch(page); m != nil {
		return html.UnescapeString(string(m[1]))
	}

	return ""
}

// tidal reads the Open Graph tags from the public page; Tidal's oEmbed
// endpoint only returns an iframe. The page title reads "Artist - Title" for
// albums and tracks.
func (f *Fetcher) tidal(ctx context.Context, ref sonos.Ref) (Info, error) {
	body, err := f.get(ctx, fmt.Sprintf("%s/%s/%s", f.TidalBase, ref.Kind, ref.ID))
	if err != nil {
		return Info{}, err
	}

	info := Info{
		Title:       metaContent(body, "og:title"),
		CoverURL:    metaContent(body, "og:image"),
		ReleaseDate: NormalizeDate(metaContent(body, "music:release_date")),
	}
	if info.Title == "" {
		return Info{}, fmt.Errorf("no title found on Tidal page")
	}

	if ref.Kind != sonos.Playlist {
		if artist, title, ok := strings.Cut(info.Title, " - "); ok {
			info.Artist, info.Title = strings.TrimSpace(artist), strings.TrimSpace(title)
		}
	}

	return info, nil
}
