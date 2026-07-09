package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"git.horn/cueBreaker/backend/web"
)

// newStaticHandler serves the embedded SPA build (web.Dist), falling back to
// index.html for any unknown path so client-side routing works. Paths under
// /api/ that reach this handler (no API route matched) get a plain 404
// instead of the SPA fallback.
func newStaticHandler() (http.Handler, error) {
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return nil, fmt.Errorf("server: sub dist fs: %w", err)
	}
	httpFS := http.FS(dist)
	fileServer := http.FileServer(httpFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		if f, err := httpFS.Open(path.Clean(r.URL.Path)); err == nil {
			_ = f.Close()
		} else if os.IsNotExist(err) {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
