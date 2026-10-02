package web

import (
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/model"
	"net/http"
)

func (s *Server) library(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch r.URL.Path {
	case "/api/library":
		if !s.service.HasStorage() {
			reply(w, 200, []any{})
			return
		}
		v, e := s.service.Library(r.Context(), app.LibraryRequest{Action: "list"})
		if e != nil {
			failure(w, 400, e)
			return
		}
		reply(w, 200, v)
	case "/api/library/detail":
		v, e := s.service.LibraryCharacter(r.Context(), q.Get("id"), q.Get("version"))
		if e != nil {
			failure(w, 404, e)
			return
		}
		reply(w, 200, v)
	case "/api/library/media":
		data, e := s.service.LibraryMedia(r.Context(), q.Get("id"), q.Get("version"), q.Get("path"))
		if e != nil {
			failure(w, 404, e)
			return
		}
		writePNG(w, data, r.URL.Query().Get("thumbnail") == "1")
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) character(w http.ResponseWriter, r *http.Request, p Project, action string) {
	if action == "character" {
		var pkg model.CharacterPackage
		if e := decode(w, r, &pkg); e != nil {
			failure(w, 400, e)
			return
		}
		path, e := s.service.CreateCharacter(r.Context(), p.Handle, pkg)
		if e != nil {
			failure(w, 409, e)
			return
		}
		reply(w, 201, map[string]string{"path": path})
		return
	}
	var body app.LibraryRequest
	if e := decode(w, r, &body); e != nil {
		failure(w, 400, e)
		return
	}
	body.ProjectFile = p.Handle
	if body.Action != "publish" && body.Action != "use" {
		failure(w, 400, fmt.Errorf("choose publish or use"))
		return
	}
	v, e := s.service.Library(r.Context(), body)
	if e != nil {
		failure(w, 409, e)
		return
	}
	s.clearHistory(p.ID)
	reply(w, 200, v)
}
