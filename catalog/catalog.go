// Package catalog lists an artist's whole discography so an admin can import
// it in one go. The public web pages only expose an artist's top tracks, so
// this uses each service's official API with free developer credentials
// (client-credentials flow, no user login).
package catalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Artist is who a discography belongs to.
type Artist struct {
	Name     string
	ImageURL string
}

// Album is one release in a discography. For a Hörspiel series each
// episode is typically one album.
type Album struct {
	ID          string
	Title       string
	Artist      string // the artist being imported, so releases group under them
	CoverURL    string
	ReleaseDate string // YYYY-MM-DD
	Type        string // "album", "single" or "compilation"
}

// Discography is everything a service lists for one artist, oldest first.
type Discography struct {
	Service string // "spotify" or "tidal"
	Artist  Artist
	Albums  []Album
}

// Provider fetches one service's discography for an artist ID.
type Provider interface {
	Discography(ctx context.Context, artistID string) (Discography, error)
}

// ErrBadLink means the link is not a Spotify or Tidal artist link.
var ErrBadLink = errors.New("not a Spotify or Tidal artist link")

// NotConfiguredError means no API credentials were given for the service.
type NotConfiguredError struct{ Service string }

func (e NotConfiguredError) Error() string {
	return fmt.Sprintf("%s API credentials are not configured on the server", e.Service)
}

var (
	spotifyArtistURL = regexp.MustCompile(`open\.spotify\.com/(?:intl-[a-z]+/)?artist/([0-9A-Za-z]+)`)
	spotifyArtistURI = regexp.MustCompile(`^spotify:artist:([0-9A-Za-z]+)$`)
	tidalArtistURL   = regexp.MustCompile(`tidal\.com/(?:browse/)?artist/(\d+)`)
)

// ParseArtistLink extracts the service ("spotify" or "tidal") and artist ID.
// It accepts artist pages, e.g. .../artist/<id>/discography/all, and spotify:artist: URIs.
func ParseArtistLink(link string) (service, id string, err error) {
	link = strings.TrimSpace(link)
	if m := spotifyArtistURL.FindStringSubmatch(link); m != nil {
		return "spotify", m[1], nil
	}
	if m := spotifyArtistURI.FindStringSubmatch(link); m != nil {
		return "spotify", m[1], nil
	}
	if m := tidalArtistURL.FindStringSubmatch(link); m != nil {
		return "tidal", m[1], nil
	}
	return "", "", ErrBadLink
}

// DisplayName is the service name as shown to people: "spotify" -> "Spotify".
func DisplayName(service string) string {
	if service == "" {
		return ""
	}
	return strings.ToUpper(service[:1]) + service[1:]
}

// Service routes an artist link to the configured provider.
type Service struct {
	Providers map[string]Provider // "spotify", "tidal"; missing means not configured
}

// Discography resolves an artist link and returns the releases oldest first.
func (s Service) Discography(ctx context.Context, link string) (Discography, error) {
	svc, id, err := ParseArtistLink(link)
	if err != nil {
		return Discography{}, err
	}
	p, ok := s.Providers[svc]
	if !ok || p == nil {
		return Discography{}, NotConfiguredError{Service: DisplayName(svc)}
	}
	d, err := p.Discography(ctx, id)
	if err != nil {
		return Discography{}, err
	}
	d.Service = svc
	sort.SliceStable(d.Albums, func(i, j int) bool {
		a, b := d.Albums[i], d.Albums[j]
		if (a.ReleaseDate == "") != (b.ReleaseDate == "") {
			return a.ReleaseDate != "" // undated last
		}
		if a.ReleaseDate != b.ReleaseDate {
			return a.ReleaseDate < b.ReleaseDate
		}
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	})
	return d, nil
}
