//go:build !dev

package http

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:spa_assets/dist
var spaFS embed.FS

func (s *Server) spaHandler() http.Handler {
	dist, _ := fs.Sub(spaFS, "spa_assets/dist")
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the exact file.
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}

		if f, err := dist.Open(p[1:]); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fall back to index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
