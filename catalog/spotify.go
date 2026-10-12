package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nimdanitro/go-sonos-kids/meta"
)

// Spotify lists discographies through the Spotify Web API. Create a free app
// at https://developer.spotify.com/dashboard to get a client ID and secret.
type Spotify struct {
	Market  string // ISO country code; only releases playable there are listed
	APIBase string // https://api.spotify.com/v1

	tokens *tokenSource

	mu    sync.Mutex
	limit int // page size in use; 0 until first needed
}

// Spotify used to allow 50 items per page. Apps created since the limit was
// lowered only get 10, and asking for more is answered with a 400. We start at 50
// and drop to 10 for good the first time that happens.
const (
	maxPageLimit = 50
	minPageLimit = 10
	maxPages     = 100 // at the smallest page size this is 1000 releases
)

func (s *Spotify) pageLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limit == 0 {
		s.limit = maxPageLimit
	}
	return s.limit
}

// lowerLimit drops to the smallest page size and reports whether that changed anything.
func (s *Spotify) lowerLimit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limit != 0 && s.limit <= minPageLimit {
		return false
	}
	s.limit = minPageLimit
	return true
}

func (s *Spotify) albumsURL(artistID string) string {
	u := fmt.Sprintf("%s/artists/%s/albums?include_groups=album,single,compilation&limit=%d", s.APIBase, url.PathEscape(artistID), s.pageLimit())
	if s.Market != "" {
		u += "&market=" + url.QueryEscape(s.Market)
	}
	return u
}

func NewSpotify(clientID, clientSecret, market string) *Spotify {
	return &Spotify{
		Market:  market,
		APIBase: "https://api.spotify.com/v1",
		tokens: &tokenSource{
			client:       &http.Client{Timeout: 15 * time.Second},
			url:          "https://accounts.spotify.com/api/token",
			clientID:     clientID,
			clientSecret: clientSecret,
		},
	}
}

type spotifyImage struct {
	URL string `json:"url"`
}

func (s *Spotify) Discography(ctx context.Context, artistID string) (Discography, error) {
	var a struct {
		Name   string         `json:"name"`
		Images []spotifyImage `json:"images"`
	}
	if err := s.tokens.getJSON(ctx, fmt.Sprintf("%s/artists/%s", s.APIBase, url.PathEscape(artistID)), "", &a); err != nil {
		return Discography{}, err
	}
	d := Discography{Artist: Artist{Name: a.Name}}
	if len(a.Images) > 0 {
		d.Artist.ImageURL = a.Images[0].URL // largest first
	}

	next := s.albumsURL(artistID)
	for page := 0; next != "" && page < maxPages; page++ {
		var res struct {
			Items []struct {
				ID          string         `json:"id"`
				Name        string         `json:"name"`
				AlbumType   string         `json:"album_type"`
				ReleaseDate string         `json:"release_date"`
				Images      []spotifyImage `json:"images"`
			} `json:"items"`
			Next string `json:"next"`
		}
		err := s.tokens.getJSON(ctx, next, "", &res)
		var he *httpError
		if errors.As(err, &he) && he.Status == http.StatusBadRequest && page == 0 && s.lowerLimit() {
			// Probably "limit too large": start over with the smaller page size.
			next = s.albumsURL(artistID)
			page--
			continue
		}
		if err != nil {
			return Discography{}, err
		}
		for _, it := range res.Items {
			al := Album{
				ID: it.ID, Title: it.Name, Artist: a.Name,
				ReleaseDate: meta.NormalizeDate(it.ReleaseDate), Type: it.AlbumType,
			}
			if len(it.Images) > 0 {
				al.CoverURL = it.Images[0].URL
			}
			d.Albums = append(d.Albums, al)
		}
		next = res.Next
	}
	return d, nil
}
