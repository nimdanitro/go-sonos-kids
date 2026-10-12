package store

import (
	"context"
	"encoding/json"
)

// Progress records how far a kid got in an album or audiobook, so playback can
// resume where they stopped. It is stored as JSON in items.progress, with a
// record per track so that a chapter list could be shown later.
type Progress struct {
	Last      int                   `json:"last"`                // track number last heard (1-based)
	Total     int                   `json:"total,omitempty"`     // tracks in the album, when known
	Completed bool                  `json:"completed,omitempty"` // the last track was heard to the end
	Tracks    map[int]TrackProgress `json:"tracks"`              // by track number
}

// TrackProgress is what is known about one track.
type TrackProgress struct {
	Title    string `json:"title,omitempty"`
	Duration int    `json:"duration,omitempty"` // seconds
	Position int    `json:"position"`           // seconds into the track when last seen
	Done     bool   `json:"done,omitempty"`     // heard (nearly) to the end
}

// ParseProgress decodes an item's stored progress. ok is false when there is
// none (never played, or reset) or it cannot be read.
func ParseProgress(raw string) (p Progress, ok bool) {
	if raw == "" || json.Unmarshal([]byte(raw), &p) != nil || p.Last < 1 {
		return Progress{}, false
	}

	return p, true
}

// Progress returns the item's progress, if any.
func (it Item) Progress() (Progress, bool) { return ParseProgress(it.ProgressJSON) }

// TracksDone counts the tracks heard to the end.
func (p Progress) TracksDone() int {
	n := 0

	for _, t := range p.Tracks {
		if t.Done {
			n++
		}
	}

	return n
}

// SetProgress saves an item's progress; nil forgets it.
func (s *Store) SetProgress(ctx context.Context, id int64, p *Progress) error {
	raw := ""

	if p != nil {
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}

		raw = string(b)
	}

	res, err := s.db.ExecContext(ctx, `UPDATE items SET progress = ? WHERE id = ?`, raw, id)

	return affected(res, err)
}
