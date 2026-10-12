package sonos

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Service is a music service linked to the Sonos household.
type Service struct {
	Name   string
	SID    string // service ID in x-sonos URIs (?sid=)
	Rincon string // service type used in the DIDL <desc>: SA_RINCON<Rincon>_X_#Svc<Rincon>-<token>-Token
}

// Confirm SIDs with MusicServices.ListAvailableServices if playback fails.
var (
	Tidal   = Service{Name: "Tidal", SID: "174", Rincon: "44551"}
	Spotify = Service{Name: "Spotify", SID: "9", Rincon: "2311"}
)

// ServiceByName maps a lowercase service name ("tidal", "spotify") to its Service.
func ServiceByName(name string) (Service, bool) {
	switch strings.ToLower(name) {
	case "tidal":
		return Tidal, true
	case "spotify":
		return Spotify, true
	}

	return Service{}, false
}

// Account holds the per-household details needed to address a service.
type Account struct {
	Service
	SN    string // account serial number (?sn=)
	Token string // account token inside the DIDL <desc>
}

// Kind is the type of content that can be queued.
type Kind string

const (
	Album    Kind = "album"
	Playlist Kind = "playlist"
	Track    Kind = "track"
)

// Ref identifies a piece of content on a music service.
type Ref struct {
	Service Service
	Kind    Kind
	ID      string
}

func (r Ref) String() string { return fmt.Sprintf("%s %s %s", r.Service.Name, r.Kind, r.ID) }

var (
	tidalURLRe   = regexp.MustCompile(`tidal\.com/(?:browse/)?(album|playlist|track)/([0-9A-Za-z-]+)`)
	spotifyURLRe = regexp.MustCompile(`open\.spotify\.com/(?:intl-[a-z]+/)?(album|playlist|track)/([0-9A-Za-z]+)`)
	spotifyURIRe = regexp.MustCompile(`^spotify:(album|playlist|track):([0-9A-Za-z]+)$`)
	numericRe    = regexp.MustCompile(`^\d+$`)
	uuidRe       = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)
)

// ParseRef accepts a tidal.com or open.spotify.com URL, a spotify: URI, a bare
// numeric Tidal album ID, or a bare Tidal playlist UUID.
func ParseRef(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	if m := tidalURLRe.FindStringSubmatch(s); m != nil {
		return Ref{Tidal, Kind(m[1]), m[2]}, nil
	}

	if m := spotifyURLRe.FindStringSubmatch(s); m != nil {
		return Ref{Spotify, Kind(m[1]), m[2]}, nil
	}

	if m := spotifyURIRe.FindStringSubmatch(s); m != nil {
		return Ref{Spotify, Kind(m[1]), m[2]}, nil
	}

	switch {
	case strings.Contains(s, "://"):
		return Ref{}, fmt.Errorf("unsupported URL %q (Tidal/Spotify albums, playlists, tracks only)", s)
	case numericRe.MatchString(s):
		return Ref{Tidal, Album, s}, nil
	case uuidRe.MatchString(s):
		return Ref{Tidal, Playlist, s}, nil
	}

	return Ref{}, fmt.Errorf("cannot tell what %q is; pass a tidal.com or open.spotify.com URL", s)
}

// Account detects the account serial number and token for a service by looking
// at existing favorites of that service (add one in the Sonos app if none).
func (s Speaker) Account(ctx context.Context, svc Service) (Account, error) {
	favs, err := s.Favorites(ctx)
	if err != nil {
		return Account{}, err
	}

	return accountFromFavorites(svc, favs)
}

func accountFromFavorites(svc Service, favs []Favorite) (Account, error) {
	sidRe := regexp.MustCompile(`[?&]sid=` + svc.SID + `(?:&|$)`)
	snRe := regexp.MustCompile(`[?&]sn=(\d+)`)
	tokenRe := regexp.MustCompile(`Svc` + svc.Rincon + `-([^-<"]+)-Token`)

	for _, f := range favs {
		if !sidRe.MatchString(f.URI) {
			continue
		}

		m := snRe.FindStringSubmatch(f.URI)
		if m == nil {
			continue
		}

		acc := Account{Service: svc, SN: m[1], Token: "0"}
		if t := tokenRe.FindStringSubmatch(f.Metadata); t != nil {
			acc.Token = t[1]
		}

		return acc, nil
	}

	return Account{}, fmt.Errorf("no %s favorite found; add a %s item to My Sonos in the Sonos app", svc.Name, svc.Name)
}

// queueItem describes what to enqueue: the URI plus candidate DIDL metadata,
// most likely first.
type queueItem struct {
	uri   string
	metas []string
}

func (a Account) queueItem(r Ref) (queueItem, error) {
	var (
		itemID, uri, class, parent string
		altMetas                   []string
	)

	didl := func(parent, class string) string {
		return fmt.Sprintf(
			`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+
				`<item id="%s" parentID="%s" restricted="true"><dc:title></dc:title>`+
				`<upnp:class>%s</upnp:class>`+
				`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON%s_X_#Svc%s-%s-Token</desc></item></DIDL-Lite>`,
			html.EscapeString(itemID),
			html.EscapeString(parent),
			class,
			a.Rincon,
			a.Rincon,
			a.Token,
		)
	}
	id := url.PathEscape(r.ID)
	query := func(flags string) string { return fmt.Sprintf("?sid=%s&flags=%s&sn=%s", a.SID, flags, a.SN) }

	switch {
	case r.Service == Tidal && r.Kind == Album: // verified against favorites
		itemID = "1004206calbum%2f" + id
		uri = "x-rincon-cpcontainer:" + itemID + query("8300")
		parent, class = "search:category:album", "object.container.album.musicAlbum.#tidalList"
	case r.Service == Tidal && r.Kind == Playlist: // inferred; two shapes tried
		itemID = "1006206cplaylist%2f" + id
		uri = "x-rincon-cpcontainer:" + itemID + query("8300")
		parent, class = itemID, "object.container.playlistContainer"

		altMetas = append(altMetas, "search:category:playlist|object.container.playlistContainer.#tidalList")
	case r.Service == Tidal && r.Kind == Track: // verified against favorites
		itemID = "10032028track%2F" + id
		uri = "x-sonos-http:track%2F" + id + ".flac" + query("8232")
		parent, class = itemID, "object.item.audioItem.musicTrack"
	case r.Service == Spotify && r.Kind == Album: // verified against favorites
		itemID = "1004006cspotify%3Aalbum%3A" + id
		uri = "x-rincon-cpcontainer:" + itemID + query("108")
		parent, class = itemID, "object.container.album.musicAlbum"
	case r.Service == Spotify && r.Kind == Playlist: // verified against favorites
		itemID = "1006206cspotify%3Aplaylist%3A" + id
		uri = "x-rincon-cpcontainer:" + itemID + query("8300")
		parent, class = itemID, "object.container.playlistContainer"
	case r.Service == Spotify && r.Kind == Track: // inferred (no track favorite to verify)
		itemID = "00032020spotify%3Atrack%3A" + id
		uri = "x-sonos-spotify:spotify%3Atrack%3A" + id + query("8232")
		parent, class = itemID, "object.item.audioItem.musicTrack"
	default:
		return queueItem{}, fmt.Errorf("unsupported: %s", r)
	}

	item := queueItem{uri: uri, metas: []string{didl(parent, class)}}

	for _, alt := range altMetas {
		p, c, _ := strings.Cut(alt, "|")
		item.metas = append(item.metas, didl(p, c))
	}

	return item, nil
}

// Play replaces the queue with the content and starts playback from the
// beginning. Call it on the group coordinator.
func (s Speaker) Play(ctx context.Context, acc Account, r Ref) error {
	return s.PlayFrom(ctx, acc, r, nil)
}

// PlayFrom is like Play but starts at resume (a track and a time in it) when
// given. Resuming is best effort: if the speaker will not seek, playback simply
// starts at the beginning of that track.
func (s Speaker) PlayFrom(ctx context.Context, acc Account, r Ref, resume *Resume) error {
	item, err := acc.queueItem(r)
	if err != nil {
		return err
	}

	if _, err := s.call(
		ctx,
		avTransportPath,
		avTransportNS,
		"RemoveAllTracksFromQueue",
		arg{"InstanceID", "0"},
	); err != nil {
		return err
	}

	var addErr error
	for _, meta := range item.metas {
		_, addErr = s.call(ctx, avTransportPath, avTransportNS, "AddURIToQueue",
			arg{"InstanceID", "0"}, arg{"EnqueuedURI", item.uri}, arg{"EnqueuedURIMetaData", meta},
			arg{"DesiredFirstTrackNumberEnqueued", "0"}, arg{"EnqueueAsNext", "0"})
		if addErr == nil {
			break
		}
	}

	if addErr != nil {
		return addErr
	}

	if _, err := s.call(
		ctx,
		avTransportPath,
		avTransportNS,
		"SetAVTransportURI",
		arg{
			"InstanceID",
			"0",
		},
		arg{"CurrentURI", "x-rincon-queue:" + s.UUID + "#0"},
		arg{"CurrentURIMetaData", ""},
	); err != nil {
		return err
	}

	if resume != nil && resume.Track > 1 {
		_ = s.seek(ctx, "TRACK_NR", strconv.Itoa(resume.Track))
	}

	if _, err = s.call(
		ctx,
		avTransportPath,
		avTransportNS,
		"Play",
		arg{"InstanceID", "0"},
		arg{"Speed", "1"},
	); err != nil {
		return err
	}

	if resume != nil && resume.Position >= time.Second {
		_ = s.seekWhenPlaying(ctx, formatClock(resume.Position))
	}

	return nil
}
