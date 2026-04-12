package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

//go:embed web/dist/index.html web/dist/assets/app.css web/dist/assets/app.js
var embeddedWebUIDist embed.FS

func webUIEnabled() bool {
	return os.Getenv("WEB_UI") == "true"
}

func webUIDistFS() fs.FS {
	dist, err := fs.Sub(embeddedWebUIDist, "web/dist")
	if err != nil {
		panic(err)
	}
	return dist
}

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	setWebUICacheHeaders(w)

	requestPath := path.Clean(r.URL.Path)
	if requestPath == "." {
		requestPath = "/"
	}

	if requestPath == "/" {
		http.ServeFileFS(w, r, webUIDistFS(), "index.html")
		return
	}

	cleaned := strings.TrimPrefix(requestPath, "/")
	if strings.HasPrefix(cleaned, "assets/") {
		http.FileServer(http.FS(webUIDistFS())).ServeHTTP(w, r)
		return
	}

	if path.Ext(cleaned) != "" {
		http.NotFound(w, r)
		return
	}

	http.ServeFileFS(w, r, webUIDistFS(), "index.html")
}

func setWebUICacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if webUIEnabled() {
		serveWebUI(w, r)
		return
	}
	if r.URL.Path != "/" {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	handleHealth(w, r)
}
