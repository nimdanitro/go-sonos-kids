package server

import (
	"context"
	"maps"
	"reflect"
	"strings"
	"time"

	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

const (
	doneMargin = 15 // seconds: a track counts as heard once it is this close to its end
	rewind     = 5  // seconds: resume this far before where the kid stopped, for context
	resumeMin  = 10 // seconds: barely started tracks are just played from the top
	stepSize   = 5  // seconds: positions are stored in steps of this, so polling does not write constantly
)

// nearEnd reports whether position is close enough to the end of a track of
// that duration to count the track as heard. Short tracks get a smaller margin.
func nearEnd(position, duration int) bool {
	if duration <= 0 {
		return false
	}

	return position >= duration-min(doneMargin, duration/10)
}

// updateProgress folds one reading of what the speaker is playing into an
// item's progress. It is pure, so the rules are easy to test.
func updateProgress(p store.Progress, st PlayerStatus) store.Progress {
	if st.Track < 1 {
		return p
	}
	// Copy the map: p may be shared with the caller's copy.
	tracks := make(map[int]store.TrackProgress, len(p.Tracks)+1)
	maps.Copy(tracks, p.Tracks)

	p.Tracks = tracks

	// Moving on from a track that was heard (nearly) to the end marks it done.
	// Skipping ahead through the "next" button does not.
	if p.Last > 0 && p.Last != st.Track {
		if prev, ok := p.Tracks[p.Last]; ok && nearEnd(prev.Position, prev.Duration) {
			prev.Done = true
			p.Tracks[p.Last] = prev
		}
	}

	cur := p.Tracks[st.Track]

	cur.Title = st.Title
	if st.DurationSec > 0 {
		cur.Duration = st.DurationSec
	}

	cur.Position = st.PositionSec / stepSize * stepSize
	if nearEnd(st.PositionSec, cur.Duration) {
		cur.Done = true
		cur.Position = st.PositionSec // keep the exact value: it is what proves the track was heard
	}

	p.Tracks[st.Track] = cur

	p.Last = st.Track
	if st.NumTracks > 0 {
		p.Total = st.NumTracks
	}

	if p.Total > 0 && st.Track == p.Total && cur.Done {
		p.Completed = true // sticky: replays start fresh progress instead
	}

	return p
}

// resumePoint says where to continue an unfinished item, or nil to start from
// the beginning (nothing heard yet, barely started, or already finished).
func resumePoint(p store.Progress) *sonos.Resume {
	if p.Completed || p.Last < 1 {
		return nil
	}

	t := p.Tracks[p.Last]
	if t.Done { // stopped between tracks: continue with the next one
		if p.Total > 0 && p.Last >= p.Total {
			return nil
		}

		return &sonos.Resume{Track: p.Last + 1}
	}

	pos := max(0, t.Position-rewind)
	if p.Last == 1 && pos < resumeMin {
		return nil
	}

	return &sonos.Resume{Track: p.Last, Position: time.Duration(pos) * time.Second}
}

// fractionOf says how far through the whole queue the speaker is, 0..1, or
// ok=false when that cannot be known (radio, or a track of unknown length).
// Tracks before the current one count as heard.
func fractionOf(st PlayerStatus) (fraction float64, ok bool) {
	if st.Track < 1 || st.DurationSec <= 0 {
		return 0, false
	}

	within := min(1, float64(st.PositionSec)/float64(st.DurationSec))
	if st.NumTracks > 0 {
		return min(1, (float64(st.Track-1)+within)/float64(st.NumTracks)), true
	}

	return within, true
}

// progressJSON is what the kids' UI needs to show how far along an item is.
type progressJSON struct {
	Track     int     `json:"track"`
	Total     int     `json:"total"`
	Position  int     `json:"position"` // seconds into that track
	Title     string  `json:"title"`
	Completed bool    `json:"completed"`
	Fraction  float64 `json:"fraction"` // 0..1 through the whole item
}

func toProgress(p store.Progress) progressJSON {
	t := p.Tracks[p.Last]
	out := progressJSON{Track: p.Last, Total: p.Total, Position: t.Position, Title: t.Title, Completed: p.Completed}

	within := 0.0
	if t.Duration > 0 {
		within = min(1, float64(t.Position)/float64(t.Duration))
	}

	switch {
	case p.Completed:
		out.Fraction = 1
	case p.Total > 0:
		out.Fraction = min(1, (float64(p.Last-1)+within)/float64(p.Total)) // tracks before Last count as heard
	default:
		out.Fraction = within
	}

	return out
}

// ---- tracking what each kid is listening to

// nowPlaying is the audiobook a profile's speaker was last told to play by this
// server. Progress is only recorded for it.
type nowPlaying struct {
	itemID   int64
	album    string // the album the speaker reports, learned from the first reading
	anchored bool
}

// startedPlaying remembers what was just started for a profile. Only
// audiobooks are tracked; starting anything else stops tracking.
func (s *Server) startedPlaying(profileID int64, it store.Item) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if it.Category != store.CategoryAudiobook {
		delete(s.playing, profileID)
		return
	}

	if s.playing == nil {
		s.playing = map[int64]*nowPlaying{}
	}

	s.playing[profileID] = &nowPlaying{itemID: it.ID}
}

// trackedItem is the audiobook this server started for the profile and that is
// still what its speaker is playing, if any. The kids' UI offers "start over"
// only for it: music and playlists have nothing to resume or restart, and an
// album started from the Sonos app is not this item any more.
func (s *Server) trackedItem(profileID int64) (itemID int64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	np := s.playing[profileID]
	if np == nil {
		return 0, false
	}

	return np.itemID, true
}

// recordProgress saves how far the profile's audiobook has got. It is called
// with each fresh reading of the speaker, which the player screen requests every
// couple of seconds, so no separate poller is needed.
func (s *Server) recordProgress(ctx context.Context, profileID int64, st PlayerStatus) {
	if st.State != "PLAYING" && st.State != "PAUSED_PLAYBACK" {
		return // stopped or starting up: positions are not meaningful
	}

	s.mu.Lock()

	np := s.playing[profileID]
	if np == nil {
		s.mu.Unlock()
		return
	}

	switch {
	case !np.anchored:
		np.album, np.anchored = st.Album, true
	case st.Album != "" && np.album != "" && !strings.EqualFold(st.Album, np.album):
		// A different album is playing: someone used the Sonos app. Stop attributing it to this item.
		delete(s.playing, profileID)
		s.mu.Unlock()

		return
	}

	itemID := np.itemID
	s.mu.Unlock()

	it, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return
	}

	before, _ := it.Progress()

	after := updateProgress(before, st)
	if reflect.DeepEqual(before, after) {
		return
	}

	_ = s.store.SetProgress(ctx, itemID, &after) // best effort
}
