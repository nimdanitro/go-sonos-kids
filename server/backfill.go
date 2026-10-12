package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

// BackfillResult says what a backfill did.
type BackfillResult struct {
	Checked int // distinct releases that were missing something and were looked up
	Updated int // library entries that were changed
}

// BackfillMetadata fills in the artist, release date and cover of saved albums
// and tracks that lack them, e.g. ones saved before the library recorded
// artists. Without an artist, music cannot be grouped by artist. Lookups are
// best effort: whatever cannot be found is left as is, and the admin can edit
// it by hand. Anything already filled in, by hand or otherwise, is never
// overwritten.
func (s *Server) BackfillMetadata(ctx context.Context) (BackfillResult, error) {
	var res BackfillResult
	items, err := s.store.ListIncomplete(ctx)
	if err != nil {
		return res, err
	}

	// One lookup per release, however many profiles have it.
	type key struct{ service, kind, id string }
	rows := map[key][]store.Item{}
	var order []key
	for _, it := range items {
		k := key{it.Service, it.Kind, it.RefID}
		if _, seen := rows[k]; !seen {
			order = append(order, k)
		}
		rows[k] = append(rows[k], it)
	}
	res.Checked = len(order)

	infos := make(map[key]meta.Info, len(order))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	lookupCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for _, k := range order {
		svc, ok := sonos.ServiceByName(k.service)
		if !ok {
			continue
		}
		ref := sonos.Ref{Service: svc, Kind: sonos.Kind(k.kind), ID: k.id}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if info, err := s.meta.Fetch(lookupCtx, ref); err == nil {
				mu.Lock()
				infos[k] = info
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	for _, k := range order {
		info, ok := infos[k]
		if !ok {
			continue
		}
		for _, it := range rows[k] {
			changed := false
			if it.Artist == "" && info.Artist != "" {
				it.Artist, changed = info.Artist, true
			}
			if it.ReleaseDate == "" && info.ReleaseDate != "" {
				it.ReleaseDate, changed = info.ReleaseDate, true
			}
			if it.CoverURL == "" && info.CoverURL != "" {
				it.CoverURL, changed = info.CoverURL, true
			}
			// Tidal titles were stored as "Artist - Title" before the artist had its own field.
			if info.Artist != "" && it.Title == info.Artist+" - "+info.Title {
				it.Title, changed = info.Title, true
			}
			if !changed {
				continue
			}
			if err := s.store.UpdateItem(ctx, it); err != nil {
				return res, err
			}
			res.Updated++
		}
	}
	return res, nil
}

// adminRefreshMetadata runs the backfill on demand (the admin page's button).
func (s *Server) adminRefreshMetadata(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, err := s.BackfillMetadata(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"checked": res.Checked, "updated": res.Updated})
}
