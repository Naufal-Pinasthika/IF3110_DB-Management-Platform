// Package web serves the React SPA built by Vite and embedded into the server.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The placeholder keeps Go builds usable for API-only development before the
// first frontend build. Vite replaces this directory with production assets.
//go:embed all:dist
var files embed.FS

func New() (http.Handler, error) {
	assets, err := fs.Sub(files, "dist")
	if err != nil {
		return nil, err
	}
	index, indexErr := fs.ReadFile(assets, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." {
			name = ""
		}
		if strings.HasPrefix(name, "api/") || name == "api" || name == ".gitkeep" || strings.HasPrefix(name, ".") {
			http.NotFound(w, r)
			return
		}
		if name != "" && name != "index.html" {
			if info, err := fs.Stat(assets, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFileFS(w, r, assets, name)
				return
			}
		}
		// Missing bundles and favicon requests must not receive HTML fallback.
		if strings.HasPrefix(name, "assets/") || name == "assets" || strings.HasPrefix(name, "static/") || name == "favicon.ico" || (path.Ext(name) != "" && !strings.HasPrefix(name, "databases/")) {
			http.NotFound(w, r)
			return
		}
		if indexErr != nil {
			http.Error(w, "The frontend is unavailable. Please contact the platform administrator.", http.StatusServiceUnavailable)
			return
		}
		// React Router resolves application routes, including direct deep links.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(index)
	}), nil
}
