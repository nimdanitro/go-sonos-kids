package sonos

import (
	"context"
	"encoding/xml"
	"html"
)

// Favorite is an entry from the Sonos "My Sonos" list (FV:2).
type Favorite struct {
	ID       string // ContentDirectory object ID, e.g. FV:2/35
	Title    string
	URI      string // <res>
	Metadata string // <r:resMD>, DIDL-Lite
}

// Favorites lists Sonos favorites. Useful to calibrate URI formats and to
// discover per-account details (serial number, metadata token).
func (s Speaker) Favorites(ctx context.Context) ([]Favorite, error) {
	body, err := s.call(ctx, contentDirPath, contentDirNS, "Browse",
		arg{"ObjectID", "FV:2"}, arg{"BrowseFlag", "BrowseDirectChildren"},
		arg{"Filter", "dc:title,res,r:resMD"}, arg{"StartingIndex", "0"},
		arg{"RequestedCount", "100"}, arg{"SortCriteria", ""})
	if err != nil {
		return nil, err
	}

	var didl struct {
		Items []struct {
			ID    string `xml:"id,attr"`
			Title string `xml:"title"`
			Res   string `xml:"res"`
			ResMD string `xml:"resMD"`
		} `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(field(body, "Result")), &didl); err != nil {
		return nil, err
	}

	var out []Favorite
	for _, it := range didl.Items {
		out = append(out, Favorite{it.ID, it.Title, it.Res, it.ResMD})
	}

	return out, nil
}

func titleTag(title string) string { return "<dc:title>" + html.EscapeString(title) + "</dc:title>" }

// RenameFavorite changes the title of the favorite with the given object ID.
// oldTitle must be the current title (Sonos compares it before updating).
func (s Speaker) RenameFavorite(ctx context.Context, id, oldTitle, newTitle string) error {
	_, err := s.call(ctx, contentDirPath, contentDirNS, "UpdateObject",
		arg{"ObjectID", id}, arg{"CurrentTagValue", titleTag(oldTitle)}, arg{"NewTagValue", titleTag(newTitle)})

	return err
}
