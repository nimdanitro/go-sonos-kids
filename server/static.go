package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/nimdanitro/go-sonos-kids/ui"
)

// spa serves the embedded React app: real files as-is (hashed assets are cached
// forever), and index.html for every other path so client-side routes such as
// a future /admin work on reload. Unknown /api/ paths stay JSON 404s.
func (s *Server) spa() http.Handler {
	sub, _ := fs.Sub(ui.Assets, ui.Build)
	files := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}

		if name := strings.TrimPrefix(path.Clean(r.URL.Path), "/"); name != "" && name != "index.html" {
			if f, err := sub.Open(name); err == nil {
				_ = f.Close()

				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}

				files.ServeHTTP(w, r)

				return
			}
		}

		if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
			w.Header().Set("X-Robots-Tag", "noindex")
		}

		index, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, "UI not built: run `go generate ./ui` (or `yarn build` in ui/)", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
