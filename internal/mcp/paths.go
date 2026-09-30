package mcp

import (
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func within(root, p string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// Resolve existing ancestors before appending absent output components.
func (s *server) path(p string) (string, error) {
	if p == "" || !filepath.IsAbs(p) {
		return "", &app.JobDiagnostic{Code: "outside_root", Message: "an absolute path within configured roots is required"}
	}
	p = filepath.Clean(p)
	for _, root := range s.roots {
		if within(root, p) {
			for current := p; within(root, current); current = filepath.Dir(current) {
				info, e := os.Lstat(current)
				if e != nil && !os.IsNotExist(e) {
					return "", e
				}
				if e == nil && info.Mode()&os.ModeIrregular != 0 {
					return "", &app.JobDiagnostic{Code: "outside_root", Message: "filesystem reparse point within configured root is unsupported"}
				}
				if filepath.Dir(current) == current {
					break
				}
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return "", e
			}
			if _, e = workspace.SafePath(root, rel); e != nil {
				return "", &app.JobDiagnostic{Code: "outside_root", Message: "symlink within configured root is unsupported"}
			}
		}
	}
	existing := p
	var tail []string
	for {
		_, e := os.Lstat(existing)
		if e == nil {
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", e
		}
		tail = append(tail, filepath.Base(existing))
		existing = parent
	}
	resolved, e := filepath.EvalSymlinks(existing)
	if e != nil {
		return "", e
	}
	for i := len(tail) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, tail[i])
	}
	for _, root := range s.roots {
		if within(root, resolved) {
			return resolved, nil
		}
	}
	return "", &app.JobDiagnostic{Code: "outside_root", Message: "path resolves outside configured roots"}
}
func readFile(p string, limit int64) ([]byte, error) {
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, fmt.Errorf("file must be regular and at most %d bytes", limit)
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > int(limit) {
		return nil, fmt.Errorf("file limit exceeded")
	}
	return b, e
}

// The MCP boundary rejects symlinks inside project metadata and assets rather
// than allowing a redirected write. Configured root aliases are canonicalized.
func (s *server) tree(root string) error {
	return checkTree(root, filepath.WalkDir)
}

func checkTree(root string, walk func(string, fs.WalkDirFunc) error) error {
	count := 0
	return walk(root, func(_ string, d os.DirEntry, e error) error {
		// Workers can remove render staging directories between enumeration and
		// descent. An absent entry has nothing to inspect; required project inputs
		// are still validated by their readers. Preserve all other scan errors.
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		count++
		if count > 100000 {
			return fmt.Errorf("project file limit exceeded")
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return &app.JobDiagnostic{Code: "outside_root", Message: "symlinks within project or export trees are unsupported"}
		}
		return nil
	})
}
func (s *server) project(p string, edits []app.DocumentEdit) (string, error) {
	p, e := s.path(p)
	if e != nil {
		return "", e
	}
	if filepath.Base(p) != "project.yaml" {
		return "", fmt.Errorf("MCP requires the owning project.yaml")
	}
	root := filepath.Dir(p)
	if e = s.tree(root); e != nil {
		return "", e
	}
	if _, e = os.Stat(filepath.Join(root, ".paneltree", "pending.json")); e == nil {
		return "", &app.JobDiagnostic{Code: "recovery_required", Message: "recover the interrupted edit with the local CLI before opening over MCP"}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	overrides := map[string]model.Document{}
	for _, edit := range edits {
		f := filepath.Join(root, edit.File)
		if filepath.IsAbs(edit.File) || !within(root, f) {
			return "", fmt.Errorf("edit must stay within owning project")
		}
		if _, e = s.path(f); e != nil {
			return "", e
		}
		overrides[f] = edit.Document
		if e = s.documentPaths(filepath.Dir(f), edit.Document); e != nil {
			return "", e
		}
	}
	// Check both current and proposed graphs before shared services may load either.
	for _, useEdits := range []bool{false, true} {
		seen := map[string]bool{}
		var visit func(string, int) error
		visit = func(f string, depth int) error {
			if depth > 64 || len(seen) > 1024 {
				return fmt.Errorf("project reference limit exceeded")
			}
			f, e := s.path(f)
			if e != nil {
				return e
			}
			if seen[f] {
				return nil
			}
			seen[f] = true
			doc, ok := overrides[f]
			if !useEdits || !ok {
				b, e := readFile(f, 1<<20)
				if e != nil {
					return e
				}
				if e = yaml.Unmarshal(b, &doc); e != nil {
					return e
				}
			}
			if e = s.documentPaths(filepath.Dir(f), doc); e != nil {
				return e
			}
			var refs []string
			if doc.Book != nil {
				refs = append(refs, doc.Book.Chapters...)
			}
			if doc.Chapter != nil {
				refs = append(refs, doc.Chapter.Pages...)
			}
			for _, ref := range refs {
				if e = visit(filepath.Join(filepath.Dir(f), ref), depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		if e = visit(p, 0); e != nil {
			return "", e
		}
	}
	state := filepath.Join(root, ".paneltree", "state.json")
	b, e := readFile(state, 8<<20)
	if e == nil {
		var states map[string]app.LayerStatus
		if e = json.Unmarshal(b, &states); e != nil {
			return "", e
		}
		for _, st := range states {
			if st.Manual != "" {
				if _, e = s.path(st.Manual); e != nil {
					return "", e
				}
			}
		}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	return p, nil
}
func (s *server) documentPaths(base string, doc model.Document) error {
	check := func(p string) error {
		if p == "" {
			return nil
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		_, e := s.path(p)
		return e
	}
	if doc.Book != nil {
		for _, p := range doc.Book.Chapters {
			if e := check(p); e != nil {
				return e
			}
		}
	}
	if doc.Chapter != nil {
		for _, p := range doc.Chapter.Pages {
			if e := check(p); e != nil {
				return e
			}
		}
	}
	var walk func([]model.Layer) error
	walk = func(ls []model.Layer) error {
		for _, l := range ls {
			if e := check(l.Mask); e != nil {
				return e
			}
			if l.Source != nil {
				if e := check(l.Source.Path); e != nil {
					return e
				}
				if e := check(l.Source.Font); e != nil {
					return e
				}
				if l.Source.Font != "" {
					if e := check(l.Source.Font + ".LICENSE"); e != nil {
						return e
					}
				}
			}
			if e := walk(l.Children); e != nil {
				return e
			}
		}
		return nil
	}
	if doc.Page != nil {
		for _, p := range doc.Page.Panels {
			if e := walk(p.Layers); e != nil {
				return e
			}
		}
	}
	return nil
}
