package web

import (
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/workspace"
	"net/http"
	"path/filepath"
	"strings"
)

func (s *Server) references(w http.ResponseWriter, r *http.Request, p Project) {
	var body app.ReferenceRequest
	if e := decode(w, r, &body); e != nil {
		failure(w, 400, e)
		return
	}
	body.ProjectFile = p.Handle
	if body.Path != "" {
		root := filepath.Dir(p.Handle)
		if strings.HasPrefix(p.Handle, "pg:") {
			root = s.cfg.StateDir
		}
		if _, e := workspace.SafePath(root, body.Path); e != nil {
			failure(w, 403, e)
			return
		}
	}
	set, e := s.service.Reference(r.Context(), body)
	if e != nil {
		failure(w, 409, e)
		return
	}
	// Frozen provider recipes stay on the server; the browser gets review metadata.
	for id, c := range set.Candidates {
		c.Recipe = nil
		c.IdeogramRecipe = nil
		set.Candidates[id] = c
	}
	for id, pub := range set.Published {
		for key, c := range pub.Candidates {
			c.Recipe = nil
			c.IdeogramRecipe = nil
			pub.Candidates[key] = c
		}
		set.Published[id] = pub
	}
	reply(w, 200, set)
}
