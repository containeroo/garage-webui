package endpoint

import (
	"bytes"
	"html"
	"io/fs"
	"net/http"
	"strings"
)

// Frontend serves the SPA with a deployment-relative base URL and static assets.
// AppKit strips the route prefix before reaching this handler, just like Kumbuka.
func Frontend(assetsFS fs.FS, prefix string) http.Handler {
	assets := http.FileServer(http.FS(assetsFS))
	index, err := fs.ReadFile(assetsFS, "index.html")
	if err != nil {
		// A missing embedded index is a build error; fail each request safely.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	notFound, err := fs.ReadFile(assetsFS, "404.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	base := strings.TrimRight(prefix, "/") + "/"
	index = replaceBase(index, base)
	notFound = replaceBase(notFound, base)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(index)

			return
		}

		if isMissingPage(r) {
			if _, err := fs.Stat(assetsFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(notFound)

				return
			}
		}

		assets.ServeHTTP(w, r)
	})
}

// replaceBase sets the deployment path in a built frontend document.
func replaceBase(document []byte, base string) []byte {
	replacement := []byte(`<base href="` + html.EscapeString(base) + `">`)
	document = bytes.Replace(document, []byte(`<base href="/" />`), replacement, 1)

	return bytes.Replace(document, []byte(`<base href="/">`), replacement, 1)
}

// isMissingPage identifies browser navigation without intercepting API requests.
func isMissingPage(r *http.Request) bool {
	return !strings.HasPrefix(r.URL.Path, "/api/") &&
		r.URL.Path != "/api" &&
		strings.Contains(r.Header.Get("Accept"), "text/html")
}
