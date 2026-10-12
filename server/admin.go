package server

import (
	"cmp"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nimdanitro/go-sonos-kids/catalog"
	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

// admin guards a handler with the bearer token. With no token configured the
// admin API is off entirely.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			writeErr(w, http.StatusServiceUnavailable, "admin disabled: start the server with an admin token")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, r)
	}
}

func (s *Server) adminSpeakers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	rooms, err := s.player.Rooms(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	type roomJSON struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	}
	out := make([]roomJSON, 0, len(rooms))
	for _, rm := range rooms {
		out = append(out, roomJSON{rm.Name, rm.Model})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- profiles

type profileReq struct {
	Name      string `json:"name"`
	Speaker   string `json:"speaker"`
	MaxVolume *int   `json:"max_volume"`
	Avatar    string `json:"avatar"`
}

func (q profileReq) toProfile() (store.Profile, string) {
	p := store.Profile{Name: strings.TrimSpace(q.Name), Speaker: strings.TrimSpace(q.Speaker), MaxVolume: 40, Avatar: q.Avatar}
	if q.MaxVolume != nil {
		p.MaxVolume = *q.MaxVolume
	}
	switch {
	case p.Name == "":
		return p, "name is required"
	case p.Speaker == "":
		return p, "speaker is required"
	case p.MaxVolume < 0 || p.MaxVolume > 100:
		return p, "max_volume must be 0-100"
	}
	return p, ""
}

func adminProfile(p store.Profile) profileJSON {
	return profileJSON{ID: p.ID, Name: p.Name, Speaker: p.Speaker, MaxVolume: p.MaxVolume, Avatar: p.Avatar}
}

func (s *Server) adminListProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListProfiles(r.Context())
	if err != nil {
		storeErr(w, err)
		return
	}
	out := make([]profileJSON, 0, len(ps))
	for _, p := range ps {
		out = append(out, adminProfile(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req profileReq
	if !readJSON(w, r, &req) {
		return
	}
	p, bad := req.toProfile()
	if bad != "" {
		writeErr(w, http.StatusBadRequest, bad)
		return
	}
	p, err := s.store.CreateProfile(r.Context(), p)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminProfile(p))
}

func (s *Server) adminUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req profileReq
	if !readJSON(w, r, &req) {
		return
	}
	p, bad := req.toProfile()
	if bad != "" {
		writeErr(w, http.StatusBadRequest, bad)
		return
	}
	p.ID = id
	if err := s.store.UpdateProfile(r.Context(), p); err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminProfile(p))
}

func (s *Server) adminDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteProfile(r.Context(), id); err != nil {
		storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminLibrary(w http.ResponseWriter, r *http.Request) {
	p, ok := s.profile(w, r)
	if !ok {
		return
	}
	items, err := s.store.ListItems(r.Context(), p.ID, "")
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toItems(items))
}

// ---- items

// defaultCategory is where a link goes when the admin did not choose one.
func defaultCategory(kind sonos.Kind) string {
	if kind == sonos.Playlist {
		return store.CategoryPlaylist
	}
	return store.CategoryMusic
}

// lookup parses a pasted link and fetches its details. The lookup is best
// effort: Info is empty when the service could not be reached.
func (s *Server) lookup(ctx context.Context, link string) (sonos.Ref, meta.Info, error) {
	ref, err := sonos.ParseRef(link)
	if err != nil {
		return ref, meta.Info{}, err
	}
	info, _ := s.meta.Fetch(ctx, ref)
	return ref, info, nil
}

func (s *Server) adminPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ref, info, err := s.lookup(r.Context(), req.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"service": strings.ToLower(ref.Service.Name), "kind": string(ref.Kind),
		"title": info.Title, "cover_url": info.CoverURL, "artist": info.Artist, "release_date": info.ReleaseDate,
		"category": defaultCategory(ref.Kind), // what the UI should preselect
	})
}

type failure struct {
	ProfileID int64  `json:"profile_id"`
	Error     string `json:"error"`
}

// uniqueIDs sorts and de-duplicates profile IDs.
func uniqueIDs(ids []int64) []int64 { return slices.Compact(slices.Sorted(slices.Values(ids))) }

// adminAddItems saves one link into the library of one or more profiles.
func (s *Server) adminAddItems(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL         string  `json:"url"`
		Category    string  `json:"category"`
		ProfileIDs  []int64 `json:"profile_ids"`
		Title       string  `json:"title"`
		CoverURL    string  `json:"cover_url"`
		Artist      string  `json:"artist"`
		ReleaseDate string  `json:"release_date"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.ProfileIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "profile_ids is required")
		return
	}
	ref, info, err := s.lookup(r.Context(), req.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Category == "" {
		req.Category = defaultCategory(ref.Kind)
	}
	if !store.ValidCategory(req.Category) {
		writeErr(w, http.StatusBadRequest, "category must be music, audiobook or playlist")
		return
	}
	// Anything the admin typed wins over the lookup.
	title := cmp.Or(strings.TrimSpace(req.Title), info.Title)
	if title == "" {
		writeErr(w, http.StatusUnprocessableEntity, "could not look up a title; please provide one")
		return
	}
	cover := cmp.Or(req.CoverURL, info.CoverURL)
	artist := cmp.Or(strings.TrimSpace(req.Artist), info.Artist)
	date := cmp.Or(meta.NormalizeDate(req.ReleaseDate), info.ReleaseDate)

	created := []itemJSON{}
	failed := []failure{}
	for _, pid := range uniqueIDs(req.ProfileIDs) {
		if _, err := s.store.GetProfile(r.Context(), pid); err != nil {
			failed = append(failed, failure{pid, "no such profile"})
			continue
		}
		it, err := s.store.AddItem(r.Context(), store.Item{
			ProfileID: pid, Category: req.Category, Title: title, CoverURL: cover, Artist: artist, ReleaseDate: date,
			Service: strings.ToLower(ref.Service.Name), Kind: string(ref.Kind), RefID: ref.ID,
		})
		switch {
		case errors.Is(err, store.ErrDuplicate):
			failed = append(failed, failure{pid, "already in this library"})
		case err != nil:
			failed = append(failed, failure{pid, err.Error()})
		default:
			created = append(created, toItem(it))
		}
	}
	status := http.StatusCreated
	if len(created) == 0 {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{"items": created, "errors": failed})
}

// ---- discography import

// adminDiscography lists everything an artist has released, so the admin can
// pick which releases to import.
func (s *Server) adminDiscography(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if s.catalog == nil {
		svc, _, err := catalog.ParseArtistLink(req.URL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "artist import is not configured on the server", "code": "not_configured", "service": catalog.DisplayName(svc),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	d, err := s.catalog.Discography(ctx, req.URL)
	var nc catalog.NotConfiguredError
	switch {
	case errors.Is(err, catalog.ErrBadLink):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.As(err, &nc):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error(), "code": "not_configured", "service": nc.Service})
		return
	case err != nil:
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	type albumJSON struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Artist      string `json:"artist"`
		CoverURL    string `json:"cover_url"`
		ReleaseDate string `json:"release_date"`
		Type        string `json:"type"`
	}
	albums := make([]albumJSON, 0, len(d.Albums))
	for _, a := range d.Albums {
		albums = append(albums, albumJSON{a.ID, a.Title, a.Artist, a.CoverURL, a.ReleaseDate, a.Type})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": d.Service,
		"artist":  map[string]string{"name": d.Artist.Name, "image_url": d.Artist.ImageURL},
		"albums":  albums,
	})
}

type batchItem struct {
	Service     string `json:"service"`
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	Artist      string `json:"artist"`
	ReleaseDate string `json:"release_date"`
}

var refIDRe = regexp.MustCompile(`^[0-9A-Za-z-]+$`)

const maxBatch = 500

// adminAddBatch saves many releases into one or more libraries at once, e.g.
// the albums picked from a discography. Already-saved ones are skipped.
func (s *Server) adminAddBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Category   string      `json:"category"`
		ProfileIDs []int64     `json:"profile_ids"`
		Items      []batchItem `json:"items"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	switch {
	case !store.ValidCategory(req.Category):
		writeErr(w, http.StatusBadRequest, "category must be music, audiobook or playlist")
		return
	case len(req.ProfileIDs) == 0:
		writeErr(w, http.StatusBadRequest, "profile_ids is required")
		return
	case len(req.Items) == 0 || len(req.Items) > maxBatch:
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("items must contain 1 to %d entries", maxBatch))
		return
	}
	items := make([]store.Item, len(req.Items))
	for i, b := range req.Items {
		svc, ok := sonos.ServiceByName(b.Service)
		kind := sonos.Kind(b.Kind)
		if !ok || (kind != sonos.Album && kind != sonos.Playlist && kind != sonos.Track) || !refIDRe.MatchString(b.ID) || strings.TrimSpace(b.Title) == "" {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("items[%d]: needs a known service, kind, id and a title", i))
			return
		}
		items[i] = store.Item{
			Category: req.Category, Title: strings.TrimSpace(b.Title), CoverURL: b.CoverURL, Artist: strings.TrimSpace(b.Artist),
			ReleaseDate: meta.NormalizeDate(b.ReleaseDate), Service: strings.ToLower(svc.Name), Kind: b.Kind, RefID: b.ID,
		}
	}
	s.fillCovers(r.Context(), items)

	var created, duplicates int
	failed := []failure{}
	for _, pid := range uniqueIDs(req.ProfileIDs) {
		if _, err := s.store.GetProfile(r.Context(), pid); err != nil {
			failed = append(failed, failure{pid, "no such profile"})
			continue
		}
		for _, it := range items {
			it.ProfileID = pid
			switch _, err := s.store.AddItem(r.Context(), it); {
			case errors.Is(err, store.ErrDuplicate):
				duplicates++
			case err != nil:
				failed = append(failed, failure{pid, err.Error()})
			default:
				created++
			}
		}
	}
	status := http.StatusCreated
	if created == 0 {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{"created": created, "duplicates": duplicates, "errors": failed})
}

// fillCovers looks up the cover for items that have none (best effort, a few
// at a time so a big import does not hammer the service, and only for as long
// as coverTimeout so a slow service cannot stall the import).
func (s *Server) fillCovers(ctx context.Context, items []store.Item) {
	ctx, cancel := context.WithTimeout(ctx, s.coverTimeout)
	defer cancel()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range items {
		if items[i].CoverURL != "" {
			continue
		}
		svc, _ := sonos.ServiceByName(items[i].Service)
		ref := sonos.Ref{Service: svc, Kind: sonos.Kind(items[i].Kind), ID: items[i].RefID}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if info, err := s.meta.Fetch(ctx, ref); err == nil {
				items[i].CoverURL = info.CoverURL
			}
		}()
	}
	wg.Wait()
}

func (s *Server) adminUpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	var req struct {
		Title       *string `json:"title"`
		CoverURL    *string `json:"cover_url"`
		Category    *string `json:"category"`
		Artist      *string `json:"artist"`
		ReleaseDate *string `json:"release_date"`
		Played      *bool   `json:"played"` // mark as played (true) or unplayed (false)
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Title != nil {
		it.Title = strings.TrimSpace(*req.Title)
	}
	if req.CoverURL != nil {
		it.CoverURL = *req.CoverURL
	}
	if req.Artist != nil {
		it.Artist = strings.TrimSpace(*req.Artist)
	}
	if req.ReleaseDate != nil {
		it.ReleaseDate = meta.NormalizeDate(*req.ReleaseDate) // "" clears it
	}
	if req.Category != nil {
		if !store.ValidCategory(*req.Category) {
			writeErr(w, http.StatusBadRequest, "category must be music, audiobook or playlist")
			return
		}
		it.Category = *req.Category
	}
	if it.Title == "" {
		writeErr(w, http.StatusBadRequest, "title must not be empty")
		return
	}
	if err := s.store.UpdateItem(r.Context(), it); err != nil {
		storeErr(w, err)
		return
	}
	if req.Played != nil {
		if err := s.store.SetPlayed(r.Context(), id, *req.Played); err != nil {
			storeErr(w, err)
			return
		}
	}
	// Answer with what is stored now, including the played state.
	fresh, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toItem(fresh))
}

func (s *Server) adminDeleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteItem(r.Context(), id); err != nil {
		storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
