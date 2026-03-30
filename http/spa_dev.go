//go:build dev

package http

import "net/http"

func (s *Server) spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html>
<body>
  <h1>DeployKit - Development Mode</h1>
  <p>The SPA is not embedded in dev mode.</p>
  <p>Run <code>make dev-frontend</code> and open <a href="http://localhost:5173">http://localhost:5173</a></p>
</body>
</html>`))
	})
}
