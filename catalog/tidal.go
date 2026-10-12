package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nimdanitro/go-sonos-kids/meta"
)

// Tidal lists discographies through the TIDAL Open API (JSON:API). Create an
// app at https://developer.tidal.com to get a client ID and secret.
//
// NOTE: written against the documented API shape but not yet exercised against
// the live service; failures surface as an error message in the admin page.
type Tidal struct {
	Country string // ISO country code, required by the API
	APIBase string // https://openapi.tidal.com/v2

	tokens *tokenSource
}

func NewTidal(clientID, clientSecret, country string) *Tidal {
	return &Tidal{
		Country: country,
		APIBase: "https://openapi.tidal.com/v2",
		tokens: &tokenSource{
			client:       &http.Client{Timeout: 15 * time.Second},
			url:          "https://auth.tidal.com/v1/oauth2/token",
			clientID:     clientID,
			clientSecret: clientSecret,
		},
	}
}

const jsonAPI = "application/vnd.api+json"

type tidalResource struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		ReleaseDate string `json:"releaseDate"`
		AlbumType   string `json:"albumType"`
		Type        string `json:"type"`
		Files       []struct {
			Href string `json:"href"`
			Meta struct {
				Width int `json:"width"`
			} `json:"meta"`
		} `json:"files"`
	} `json:"attributes"`
	Relationships struct {
		CoverArt struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"coverArt"`
	} `json:"relationships"`
}

func (t *Tidal) Discography(ctx context.Context, artistID string) (Discography, error) {
	id := url.PathEscape(artistID)
	country := url.QueryEscape(t.Country)

	var a struct {
		Data tidalResource `json:"data"`
	}
	if err := t.tokens.getJSON(ctx, fmt.Sprintf("%s/artists/%s?countryCode=%s", t.APIBase, id, country), jsonAPI, &a); err != nil {
		return Discography{}, err
	}
	d := Discography{Artist: Artist{Name: a.Data.Attributes.Name}}

	next := fmt.Sprintf("%s/artists/%s/relationships/albums?countryCode=%s&include=albums,albums.coverArt", t.APIBase, id, country)
	for page := 0; next != "" && page < 40; page++ {
		var res struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Included []tidalResource `json:"included"`
			Links    struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := t.tokens.getJSON(ctx, next, jsonAPI, &res); err != nil {
			return Discography{}, err
		}
		albums := map[string]tidalResource{}
		covers := map[string]string{}
		for _, inc := range res.Included {
			switch inc.Type {
			case "albums":
				albums[inc.ID] = inc
			case "artworks":
				best, width := "", -1
				for _, f := range inc.Attributes.Files {
					if f.Meta.Width > width {
						best, width = f.Href, f.Meta.Width
					}
				}
				covers[inc.ID] = best
			}
		}
		for _, ref := range res.Data {
			al, ok := albums[ref.ID]
			if !ok {
				continue
			}
			out := Album{
				ID: ref.ID, Title: al.Attributes.Title, Artist: d.Artist.Name,
				ReleaseDate: meta.NormalizeDate(al.Attributes.ReleaseDate), Type: tidalType(al),
			}
			if len(al.Relationships.CoverArt.Data) > 0 {
				out.CoverURL = covers[al.Relationships.CoverArt.Data[0].ID]
			}
			d.Albums = append(d.Albums, out)
		}
		next = t.absolute(res.Links.Next)
	}
	return d, nil
}

// absolute turns a "links.next" value, which may be relative, into a full URL.
func (t *Tidal) absolute(next string) string {
	switch {
	case next == "":
		return ""
	case strings.HasPrefix(next, "http"):
		return next
	}
	base, err := url.Parse(t.APIBase)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(next, base.Path+"/") { // already includes /v2
		return base.Scheme + "://" + base.Host + next
	}
	return t.APIBase + next
}

func tidalType(al tidalResource) string {
	kind := strings.ToUpper(al.Attributes.AlbumType)
	if kind == "" {
		kind = strings.ToUpper(al.Attributes.Type)
	}
	if kind == "SINGLE" {
		return "single"
	}
	return "album" // ALBUM, EP and anything unknown
}
