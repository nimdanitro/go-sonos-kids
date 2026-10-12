// Package server is the JSON API for the kids' music UI, plus a hidden,
// token-protected admin API and page for curating each profile's library.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/nimdanitro/go-sonos-kids/catalog"
	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/sonos"
	"github.com/nimdanitro/go-sonos-kids/store"
)

// MetaFetcher looks up title and cover art for a link.
type MetaFetcher interface {
	Fetch(ctx context.Context, ref sonos.Ref) (meta.Info, error)
}

// Catalog lists an artist's discography for the admin's bulk import.
type Catalog interface {
	Discography(ctx context.Context, link string) (catalog.Discography, error)
}

type Options struct {
	Store      *store.Store
	Player     Player
	Meta       MetaFetcher
	Catalog    Catalog // optional: without it, artist import answers "not configured"
	AdminToken string  // admin API is disabled while empty

	// CoverTimeout bounds the cover lookups a bulk import may do; releases
	// still without a cover afterwards are saved without one. Default 8s.
	CoverTimeout time.Duration
}

type Server struct {
	store   *store.Store
	player  Player
	meta    MetaFetcher
	catalog Catalog
	token   string
	mux     *http.ServeMux

	coverTimeout time.Duration

	mu      sync.Mutex
	playing map[int64]*nowPlaying // by profile ID; see progress.go
}

func New(o Options) *Server {
	s := &Server{store: o.Store, player: o.Player, meta: o.Meta, catalog: o.Catalog, token: o.AdminToken, mux: http.NewServeMux(), coverTimeout: o.CoverTimeout}
	if s.coverTimeout == 0 {
		s.coverTimeout = 8 * time.Second
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Kids' API: no searching, only what the admin put in their library.
	m.HandleFunc("GET /api/profiles", s.listProfiles)
	m.HandleFunc("GET /api/profiles/{id}/library", s.library)
	m.HandleFunc("GET /api/profiles/{id}/status", s.status)
	m.HandleFunc("POST /api/profiles/{id}/play", s.play)
	m.HandleFunc("POST /api/profiles/{id}/volume", s.volume)
	for _, action := range []string{"pause", "resume", "next", "previous"} {
		m.HandleFunc("POST /api/profiles/{id}/"+action, s.control(action))
	}

	// Hidden admin API; every call needs the token. The admin page itself is a
	// route of the React app, served by the SPA fallback below.
	m.HandleFunc("GET /api/admin/speakers", s.admin(s.adminSpeakers))
	m.HandleFunc("GET /api/admin/profiles", s.admin(s.adminListProfiles))
	m.HandleFunc("POST /api/admin/profiles", s.admin(s.adminCreateProfile))
	m.HandleFunc("PUT /api/admin/profiles/{id}", s.admin(s.adminUpdateProfile))
	m.HandleFunc("DELETE /api/admin/profiles/{id}", s.admin(s.adminDeleteProfile))
	m.HandleFunc("GET /api/admin/profiles/{id}/library", s.admin(s.adminLibrary))
	m.HandleFunc("POST /api/admin/preview", s.admin(s.adminPreview))
	m.HandleFunc("POST /api/admin/items", s.admin(s.adminAddItems))
	m.HandleFunc("POST /api/admin/items/batch", s.admin(s.adminAddBatch))
	m.HandleFunc("POST /api/admin/items/refresh", s.admin(s.adminRefreshMetadata))
	m.HandleFunc("POST /api/admin/discography", s.admin(s.adminDiscography))
	m.HandleFunc("PUT /api/admin/items/{id}", s.admin(s.adminUpdateItem))
	m.HandleFunc("DELETE /api/admin/items/{id}", s.admin(s.adminDeleteItem))

	// Everything else is the React app.
	m.Handle("GET /", s.spa())
}

// ---- JSON shapes

type profileJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Speaker   string `json:"speaker,omitempty"` // admin only
	MaxVolume int    `json:"max_volume"`
	Avatar    string `json:"avatar"`
}

type itemJSON struct {
	ID          int64  `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	CoverURL    string `json:"cover_url"`
	Artist      string `json:"artist"`
	ReleaseDate string `json:"release_date"`
	Played      bool   `json:"played"`    // started at least once by this kid
	PlayedAt    string `json:"played_at"` // when it was last started (RFC3339), or ""
	// How far the kid got, for audiobooks that were started.
	Progress *progressJSON `json:"progress,omitempty"`
	Service  string        `json:"service"`
	Kind     string        `json:"kind"`
}

func toItem(it store.Item) itemJSON {
	out := itemJSON{ID: it.ID, Category: it.Category, Title: it.Title, CoverURL: it.CoverURL, Artist: it.Artist,
		ReleaseDate: it.ReleaseDate, Played: it.PlayedAt != "", PlayedAt: it.PlayedAt, Service: it.Service, Kind: it.Kind}
	if p, ok := it.Progress(); ok && it.Category == store.CategoryAudiobook {
		pj := toProgress(p)
		out.Progress = &pj
	}
	return out
}

func toItems(items []store.Item) []itemJSON {
	out := make([]itemJSON, 0, len(items))
	for _, it := range items {
		out = append(out, toItem(it))
	}
	return out
}

// ---- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// storeErr maps store errors to HTTP responses.
func storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, "already exists")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

// profile loads the profile named in the path, writing the error response itself.
func (s *Server) profile(w http.ResponseWriter, r *http.Request) (store.Profile, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.Profile{}, false
	}
	p, err := s.store.GetProfile(r.Context(), id)
	if err != nil {
		storeErr(w, err)
		return store.Profile{}, false
	}
	return p, true
}

func refOf(it store.Item) (sonos.Ref, bool) {
	svc, ok := sonos.ServiceByName(it.Service)
	return sonos.Ref{Service: svc, Kind: sonos.Kind(it.Kind), ID: it.RefID}, ok
}
