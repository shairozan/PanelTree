package web

import (
	"embed"
	"net/http"
	"path"
)

//go:embed assets/*
var assets embed.FS

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := "assets/index.html"
	if r.URL.Path != "/" {
		name = path.Clean(r.URL.Path[1:])
		if name != "assets/app.js" && name != "assets/style.css" {
			http.NotFound(w, r)
			return
		}
	}
	data, e := assets.ReadFile(name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	kind := "text/html; charset=utf-8"
	if name == "assets/app.js" {
		kind = "text/javascript; charset=utf-8"
	}
	if name == "assets/style.css" {
		kind = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", kind)
	_, _ = w.Write(data)
}
