package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// uiAvailable reports whether a built UI is present at the configured path.
func (a *AcmednsAPI) uiAvailable() bool {
	info, err := os.Stat(a.Config.API.UIPath)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(a.Config.API.UIPath, "index.html"))
	return err == nil
}

// spaFileServer serves the built single page application from root. Existing
// files are served as-is; any other path without a file extension falls back to
// index.html so the Angular router can take over. Requests for a missing asset
// still get a real 404 instead of an HTML page, which keeps broken bundles
// obvious rather than silently returning index.html to a <script> tag.
func (a *AcmednsAPI) spaFileServer(root string) http.Handler {
	fileServer := http.FileServer(http.Dir(root))
	index := filepath.Join(root, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleaned := path.Clean("/" + r.URL.Path)

		// The API namespace never falls back to the SPA: a call to a disabled or
		// misspelled management endpoint has to answer with JSON, otherwise the
		// UI would try to parse index.html as a response.
		if strings.HasPrefix(cleaned, "/api/") || cleaned == "/api" {
			message := "unknown_endpoint"
			if !a.adminEnabled() {
				message = "management_api_disabled"
			}
			writeJSONError(w, http.StatusNotFound, message)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		candidate := filepath.Join(root, filepath.FromSlash(cleaned))

		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			// Only the content hashed bundles are safe to pin for a year.
			// Unhashed assets such as favicon.ico must stay revalidatable.
			switch path.Ext(cleaned) {
			case ".js", ".css":
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			default:
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		if path.Ext(cleaned) != "" {
			a.Logger.Debugw("UI asset not found",
				"path", cleaned)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
