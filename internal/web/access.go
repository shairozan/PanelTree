package web

import (
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) filePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("choose an absolute path inside a project root")
	}
	for _, root := range s.cfg.Roots {
		rel, e := filepath.Rel(root, path)
		if e != nil || rel == "." {
			continue
		}
		if p, e := workspace.SafePath(root, rel); e == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("path outside configured project roots")
}
func (s *Server) guardProject(handle string) error {
	if strings.HasPrefix(handle, "pg:") {
		if !s.service.HasStorage() {
			return fmt.Errorf("database not configured")
		}
		return nil
	}
	entry, e := s.filePath(handle)
	if e != nil {
		return e
	}
	if filepath.Base(entry) != "project.yaml" {
		return fmt.Errorf("register the owning project.yaml")
	}
	root := filepath.Dir(entry)
	count := 0
	if e = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		count++
		if count > 100000 {
			return fmt.Errorf("project file limit exceeded")
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("linked project paths are not allowed")
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "cache") {
			return filepath.SkipDir
		}
		return nil
	}); e != nil {
		return e
	}
	if _, e = os.Stat(filepath.Join(root, ".paneltree/pending.json")); e == nil {
		return fmt.Errorf("recover interrupted project with CLI before opening")
	} else if !os.IsNotExist(e) {
		return e
	}
	seen := map[string]bool{}
	var visit func(string, int) error
	visit = func(rel string, depth int) error {
		if depth > 64 || len(seen) > 1024 {
			return fmt.Errorf("project depth limit")
		}
		path, e := workspace.SafePath(root, rel)
		if e != nil {
			return e
		}
		if seen[path] {
			return nil
		}
		seen[path] = true
		st, e := os.Stat(path)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() || st.Size() > 1<<20 {
			return fmt.Errorf("project document exceeds limit")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var d model.Document
		if e = yaml.Unmarshal(b, &d); e != nil {
			return e
		}
		if e = storage.ValidateDocumentPaths(root, rel, d); e != nil {
			return e
		}
		var refs []string
		if d.Book != nil {
			refs = d.Book.Chapters
		}
		if d.Chapter != nil {
			refs = d.Chapter.Pages
		}
		for _, ref := range refs {
			if e = visit(filepath.Join(filepath.Dir(rel), ref), depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	if e = visit("project.yaml", 0); e != nil {
		return e
	}
	state, e := os.ReadFile(filepath.Join(root, ".paneltree/state.json"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if len(state) > 8<<20 {
		return fmt.Errorf("editorial state exceeds limit")
	}
	var states map[string]app.LayerStatus
	if e = json.Unmarshal(state, &states); e != nil {
		return e
	}
	for _, st := range states {
		if st.Manual != "" {
			path, e := asset.ManualPath(root, st.Manual)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			if _, e = workspace.SafePath(root, rel); e != nil {
				return fmt.Errorf("import external manual artwork into portable project before opening")
			}
		}
	}
	return nil
}
