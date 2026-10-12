package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

// Player is what the server needs from the Sonos household. The real
// implementation talks to speakers; tests use a fake.
type Player interface {
	// Play replaces the queue of the room's group and starts playing, from
	// resume when given (best effort). If the room is louder than maxVolume it
	// is turned down first.
	Play(ctx context.Context, room string, ref sonos.Ref, maxVolume int, resume *sonos.Resume) error
	// Control is one of "pause", "resume", "next", "previous".
	Control(ctx context.Context, room, action string) error
	SetVolume(ctx context.Context, room string, level int) error
	Status(ctx context.Context, room string) (PlayerStatus, error)
	// Rooms lists the room names available for profile mapping.
	Rooms(ctx context.Context) ([]RoomInfo, error)
}

type PlayerStatus struct {
	State    string
	Title    string
	Artist   string
	Album    string
	CoverURL string
	Volume   int

	Track       int // 1-based position in the queue
	NumTracks   int // tracks in the queue, 0 if unknown
	PositionSec int // seconds into the current track
	DurationSec int // length of the current track, 0 if unknown
}

type RoomInfo struct {
	Name  string
	Model string
}

// SonosPlayer is the real Player.
type SonosPlayer struct {
	Logf func(format string, args ...any)

	mu       sync.Mutex
	accounts map[string]sonos.Account // by service name, learned from favorites
}

func (p *SonosPlayer) groups(ctx context.Context) ([]sonos.Group, error) {
	return sonos.DiscoverGroups(ctx, sonos.DiscoverOptions{Logf: p.Logf})
}

func (p *SonosPlayer) group(ctx context.Context, room string) (sonos.Group, error) {
	groups, err := p.groups(ctx)
	if err != nil {
		return sonos.Group{}, err
	}
	for _, g := range groups {
		if g.Matches(room) {
			return g, nil
		}
	}
	return sonos.Group{}, fmt.Errorf("speaker %q not found on the network", room)
}

func (p *SonosPlayer) account(ctx context.Context, sp sonos.Speaker, svc sonos.Service) (sonos.Account, error) {
	p.mu.Lock()
	acc, ok := p.accounts[svc.Name]
	p.mu.Unlock()
	if ok {
		return acc, nil
	}
	acc, err := sp.Account(ctx, svc)
	if err != nil {
		return sonos.Account{}, err
	}
	p.mu.Lock()
	if p.accounts == nil {
		p.accounts = map[string]sonos.Account{}
	}
	p.accounts[svc.Name] = acc
	p.mu.Unlock()
	return acc, nil
}

func (p *SonosPlayer) Play(ctx context.Context, room string, ref sonos.Ref, maxVolume int, resume *sonos.Resume) error {
	g, err := p.group(ctx, room)
	if err != nil {
		return err
	}
	acc, err := p.account(ctx, g.Coordinator, ref.Service)
	if err != nil {
		return err
	}
	member := g.MemberFor(room)
	if v, err := member.Volume(ctx); err == nil && v > maxVolume {
		if err := member.SetVolume(ctx, maxVolume); err != nil {
			return err
		}
	}
	return g.Coordinator.PlayFrom(ctx, acc, ref, resume)
}

func (p *SonosPlayer) Control(ctx context.Context, room, action string) error {
	g, err := p.group(ctx, room)
	if err != nil {
		return err
	}
	c := g.Coordinator
	switch action {
	case "pause":
		return c.Pause(ctx)
	case "resume":
		return c.Resume(ctx)
	case "next":
		return c.Next(ctx)
	case "previous":
		return c.Previous(ctx)
	}
	return fmt.Errorf("unknown action %q", action)
}

func (p *SonosPlayer) SetVolume(ctx context.Context, room string, level int) error {
	g, err := p.group(ctx, room)
	if err != nil {
		return err
	}
	return g.MemberFor(room).SetVolume(ctx, level)
}

func (p *SonosPlayer) Status(ctx context.Context, room string) (PlayerStatus, error) {
	g, err := p.group(ctx, room)
	if err != nil {
		return PlayerStatus{}, err
	}
	st, err := g.Coordinator.Status(ctx)
	if err != nil {
		return PlayerStatus{}, err
	}
	vol, _ := g.MemberFor(room).Volume(ctx)
	return PlayerStatus{
		State: st.State, Title: st.Title, Artist: st.Artist, Album: st.Album, CoverURL: st.ArtURI, Volume: vol,
		Track: st.Track, NumTracks: st.NumTracks, PositionSec: st.PositionSec, DurationSec: st.DurationSec,
	}, nil
}

func (p *SonosPlayer) Rooms(ctx context.Context) ([]RoomInfo, error) {
	groups, err := p.groups(ctx)
	if err != nil {
		return nil, err
	}
	var out []RoomInfo
	for _, g := range groups {
		for _, m := range g.Members {
			out = append(out, RoomInfo{Name: m.Room, Model: m.Model})
		}
	}
	return out, nil
}

// requestTimeout bounds each Sonos interaction so a dead speaker can't hang the UI.
const requestTimeout = 15 * time.Second
