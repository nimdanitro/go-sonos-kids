package server

import (
	"context"
	"net/http"

	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListProfiles(r.Context())
	if err != nil {
		storeErr(w, err)
		return
	}

	out := make([]profileJSON, 0, len(ps))
	for _, p := range ps {
		out = append(out, profileJSON{ID: p.ID, Name: p.Name, MaxVolume: p.MaxVolume, Avatar: p.Avatar})
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) library(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profile(w, r)
	if !ok {
		return
	}

	cat := r.URL.Query().Get("category")
	if cat != "" && !store.ValidCategory(cat) {
		writeErr(w, http.StatusBadRequest, "category must be music, audiobook or playlist")
		return
	}

	items, err := s.store.ListItems(r.Context(), p.ID, cat)
	if err != nil {
		storeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toItems(items))
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profile(w, r)
	if !ok {
		return
	}

	var req struct {
		ItemID  int64 `json:"item_id"`
		Restart bool  `json:"restart"` // start an audiobook over instead of resuming it
	}
	if !readJSON(w, r, &req) {
		return
	}

	it, err := s.store.GetItem(r.Context(), req.ItemID)
	if err != nil {
		storeErr(w, err)
		return
	}
	// A kid may only play what is in their own library.
	if it.ProfileID != p.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	ref, ok := refOf(it)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "unknown service "+it.Service)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	// An unfinished audiobook continues where the kid stopped.
	var resume *sonos.Resume

	if it.Category == store.CategoryAudiobook && !req.Restart {
		if prog, ok := it.Progress(); ok {
			resume = resumePoint(prog)
		}
	}

	if err := s.player.Play(ctx, p.Speaker, ref, p.MaxVolume, resume); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	// Remember that this kid has played it (only once playback really started).
	// Best effort: failing to record it must not fail the play itself.
	_ = s.store.SetPlayed(r.Context(), it.ID, true)
	if it.Category == store.CategoryAudiobook && resume == nil {
		_ = s.store.SetProgress(r.Context(), it.ID, nil) // starting over: forget the earlier run
	}

	s.startedPlaying(p.ID, it)
	writeJSON(w, http.StatusOK, map[string]any{"status": "playing", "title": it.Title, "resumed": resume != nil})
}

func (s *Server) control(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.profile(w, r)
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()

		if err := s.player.Control(ctx, p.Speaker, action); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// volume sets the room volume, clamped to the profile's maximum.
func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profile(w, r)
	if !ok {
		return
	}

	var req struct {
		Level *int `json:"level"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if req.Level == nil {
		writeErr(w, http.StatusBadRequest, "level is required")
		return
	}

	level := max(0, min(*req.Level, p.MaxVolume))

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	if err := s.player.SetVolume(ctx, p.Speaker, level); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"level": level, "max_volume": p.MaxVolume})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profile(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	st, err := s.player.Status(ctx, p.Speaker)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	s.recordProgress(r.Context(), p.ID, st) // remember how far an audiobook has got

	out := map[string]any{
		"state": st.State, "title": st.Title, "artist": st.Artist, "album": st.Album,
		"cover_url": st.CoverURL, "volume": st.Volume, "max_volume": p.MaxVolume,
		"track": st.Track, "num_tracks": st.NumTracks, "position": st.PositionSec, "duration": st.DurationSec,
	}
	if f, ok := fractionOf(st); ok {
		out["fraction"] = f // how far through the whole album or queue, for the cover's progress bar
	}

	if id, ok := s.trackedItem(p.ID); ok {
		out["item_id"] = id // the audiobook that is playing, which can be restarted (see trackedItem)
	}

	writeJSON(w, http.StatusOK, out)
}
