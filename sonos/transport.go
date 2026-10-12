package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	renderingPath = "/MediaRenderer/RenderingControl/Control"
	renderingNS   = "urn:schemas-upnp-org:service:RenderingControl:1"
)

func (s Speaker) av(ctx context.Context, action string, extra ...arg) ([]byte, error) {
	args := append([]arg{{"InstanceID", "0"}}, extra...)
	return s.call(ctx, avTransportPath, avTransportNS, action, args...)
}

// Transport controls. Call them on the group coordinator.

func (s Speaker) Pause(ctx context.Context) error    { _, err := s.av(ctx, "Pause"); return err }
func (s Speaker) Next(ctx context.Context) error     { _, err := s.av(ctx, "Next"); return err }
func (s Speaker) Previous(ctx context.Context) error { _, err := s.av(ctx, "Previous"); return err }

// Resume starts or resumes playback of whatever is loaded.
func (s Speaker) Resume(ctx context.Context) error {
	_, err := s.av(ctx, "Play", arg{"Speed", "1"})
	return err
}

// Volume returns the room's volume (0-100). Call it on the room's own speaker.
func (s Speaker) Volume(ctx context.Context) (int, error) {
	body, err := s.call(ctx, renderingPath, renderingNS, "GetVolume", arg{"InstanceID", "0"}, arg{"Channel", "Master"})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(field(body, "CurrentVolume")))
}

// SetVolume sets the room's volume, clamped to 0-100.
func (s Speaker) SetVolume(ctx context.Context, level int) error {
	level = max(0, min(100, level))
	_, err := s.call(ctx, renderingPath, renderingNS, "SetVolume",
		arg{"InstanceID", "0"}, arg{"Channel", "Master"}, arg{"DesiredVolume", strconv.Itoa(level)})
	return err
}

// Status is what a speaker is currently doing.
type Status struct {
	State    string // PLAYING, PAUSED_PLAYBACK, STOPPED, TRANSITIONING
	Title    string
	Artist   string
	Album    string
	ArtURI   string
	Position string // h:mm:ss
	Duration string

	Track       int // 1-based position of the current track in the queue
	NumTracks   int // tracks in the queue, 0 if unknown
	PositionSec int // seconds into the current track
	DurationSec int // length of the current track in seconds, 0 if unknown
}

// parseClock turns "h:mm:ss" into seconds; anything else (e.g. NOT_IMPLEMENTED) is 0.
func parseClock(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0
	}
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		total = total*60 + n
	}
	return total
}

// formatClock is the inverse of parseClock.
func formatClock(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
}

// Status reports transport state and the current track. Call it on the group coordinator.
func (s Speaker) Status(ctx context.Context) (Status, error) {
	var st Status
	body, err := s.av(ctx, "GetTransportInfo")
	if err != nil {
		return st, err
	}
	st.State = field(body, "CurrentTransportState")

	body, err = s.av(ctx, "GetPositionInfo")
	if err != nil {
		return st, err
	}
	st.Position = field(body, "RelTime")
	st.Duration = field(body, "TrackDuration")
	st.Track, _ = strconv.Atoi(strings.TrimSpace(field(body, "Track")))
	st.PositionSec = parseClock(st.Position)
	st.DurationSec = parseClock(st.Duration)

	var didl struct {
		Item struct {
			Title   string `xml:"title"`
			Creator string `xml:"creator"`
			Album   string `xml:"album"`
			Art     string `xml:"albumArtURI"`
		} `xml:"item"`
	}
	if md := field(body, "TrackMetaData"); strings.HasPrefix(md, "<") {
		if err := xml.Unmarshal([]byte(md), &didl); err != nil {
			return st, fmt.Errorf("parse track metadata: %w", err)
		}
	}
	st.Title, st.Artist, st.Album, st.ArtURI = didl.Item.Title, didl.Item.Creator, didl.Item.Album, didl.Item.Art
	if info, err := s.av(ctx, "GetMediaInfo"); err == nil { // best effort: radio has no queue size
		st.NumTracks, _ = strconv.Atoi(strings.TrimSpace(field(info, "NrTracks")))
	}
	if strings.HasPrefix(st.ArtURI, "/") {
		st.ArtURI = s.baseURL() + st.ArtURI
	}
	return st, nil
}

// Resume says where playback should start: a track in the queue and a time in it.
type Resume struct {
	Track    int // 1-based; values below 2 mean "from the first track"
	Position time.Duration
}

func (s Speaker) seek(ctx context.Context, unit, target string) error {
	_, err := s.av(ctx, "Seek", arg{"Unit", unit}, arg{"Target", target})
	return err
}

// seekWhenPlaying seeks within the current track. The speaker only accepts
// that once playback has actually begun, so retry briefly while it starts up.
func (s Speaker) seekWhenPlaying(ctx context.Context, target string) error {
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		if err = s.seek(ctx, "REL_TIME", target); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return err
}
