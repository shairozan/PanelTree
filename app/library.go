package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/characters"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
)

type LibraryRequest struct {
	Action           string         `json:"action"`
	ProjectFile      string         `json:"project_file,omitempty"`
	ID               string         `json:"id,omitempty"`
	Version          string         `json:"version,omitempty"`
	Package          string         `json:"package,omitempty"`
	Set              string         `json:"set,omitempty"`
	ReferenceVersion string         `json:"reference_version,omitempty"`
	ExpectedRevision model.Revision `json:"expected_revision,omitempty"`
	Target           LayerTarget    `json:"target"`
}

func (s *Service) Library(ctx context.Context, r LibraryRequest) ([]storage.LibraryVersion, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("PostgreSQL library is not configured")
	}
	switch r.Action {
	case "list":
		return s.storage.Library(ctx)
	case "delete":
		return nil, s.storage.DeleteLibrary(ctx, r.ID, r.Version)
	case "publish":
		e := s.openWorkspace(ctx, r.ProjectFile, func(w *workspace.Session) error {
			if _, e := characters.Resolve(ctx, w.Root, &model.CharacterUse{Package: r.Package}); e != nil {
				return e
			}
			path, e := workspace.SafePath(w.Root, r.Package)
			if e != nil {
				return e
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var pkg model.CharacterPackage
			if e = yaml.Unmarshal(data, &pkg); e != nil {
				return e
			}
			prefix := "characters/library/" + r.ID + "/" + r.Version + "/"
			files := map[string][]byte{}
			v := storage.LibraryVersion{ID: r.ID, Version: r.Version, Package: prefix + "character.json"}
			refs := func(items []model.CharacterReference) error {
				for i := range items {
					path, e := workspace.SafePath(w.Root, items[i].Path)
					if e != nil {
						return e
					}
					data, e := os.ReadFile(path)
					if e != nil {
						return e
					}
					if len(data) > 32<<20 {
						return fmt.Errorf("character asset too large")
					}
					newPath := prefix + "assets/" + hash(items[i].Path) + filepath.Ext(items[i].Path)
					files[newPath] = data
					items[i].Path = newPath
				}
				return nil
			}
			if e = refs(pkg.References); e != nil {
				return e
			}
			if e = refs(pkg.Props); e != nil {
				return e
			}
			for _, states := range [][]model.CharacterState{pkg.Costumes, pkg.Expressions, pkg.Poses} {
				for i := range states {
					if e = refs(states[i].References); e != nil {
						return e
					}
				}
			}
			data, e = json.Marshal(pkg)
			if e != nil {
				return e
			}
			files[v.Package] = data
			if r.Set != "" {
				set, e := loadReference(w.Root, r.Set)
				if e != nil {
					return e
				}
				pub, ok := set.Published[r.ReferenceVersion]
				if !ok {
					return fmt.Errorf("select a published reference version")
				}
				set = ReferenceSet{ID: "lib-" + hash(r.ID + "/" + r.Version)[:24], Sequence: 1, Accepted: pub.Accepted, Candidates: pub.Candidates, Published: map[string]ReferencePublication{r.ReferenceVersion: pub}}
				set.Revision = hash(set)
				data, e = json.Marshal(set)
				if e != nil {
					return e
				}
				files[".paneltree/references/"+set.ID+"/00000001.json"] = data
				v.ReferenceSet = set.ID
				v.ReferenceVersion = r.ReferenceVersion
				hashes := map[string]bool{}
				for _, c := range pub.Candidates {
					hashes[c.Image] = true
				}
				for _, h := range pub.Cards {
					hashes[h] = true
				}
				for h := range hashes {
					rel := ".paneltree/assets/" + h + ".png"
					path, e := workspace.SafePath(w.Root, rel)
					if e != nil {
						return e
					}
					data, e := os.ReadFile(path)
					if e != nil {
						return e
					}
					files[rel] = data
				}
			}
			return s.storage.PublishLibrary(ctx, v, files)
		})
		return nil, e
	case "use":
		if !strings.HasPrefix(r.ProjectFile, "pg:") {
			return nil, fmt.Errorf("library use requires a PostgreSQL project; use export for a portable file copy")
		}
		e := s.storage.BindLibrary(ctx, strings.TrimPrefix(r.ProjectFile, "pg:"), r.ID, r.Version, func(w *workspace.Session, v storage.LibraryVersion) error {
			if r.ExpectedRevision == "" || w.Snapshot.Revision != r.ExpectedRevision {
				return fmt.Errorf("revision conflict")
			}
			var edits []DocumentEdit
			found := false
			var change func([]model.Layer)
			change = func(layers []model.Layer) {
				for i := range layers {
					l := &layers[i]
					if l.ID == r.Target.Layer && l.Source != nil {
						use := model.CharacterUse{}
						if l.Source.Character != nil {
							use = *l.Source.Character
						}
						use.Package = v.Package
						if v.ReferenceSet != "" {
							selection := model.ReferenceSelection{Directions: []string{"front"}, Packing: "cards-row/v1"}
							if use.ReferenceSet != nil {
								selection = *use.ReferenceSet
							}
							selection.Set = v.ReferenceSet
							selection.Version = v.ReferenceVersion
							use.ReferenceSet = &selection
						} else {
							use.ReferenceSet = nil
						}
						l.Source.Character = &use
						found = true
					}
					change(l.Children)
				}
			}
			for _, d := range w.Snapshot.Documents {
				data, e := json.Marshal(d.Document)
				if e != nil {
					return e
				}
				var copy model.Document
				if e = json.Unmarshal(data, &copy); e != nil {
					return e
				}
				d.Document = copy
				if d.Document.Page == nil || d.Document.Page.ID != r.Target.Page {
					continue
				}
				for i := range d.Document.Page.Panels {
					if d.Document.Page.Panels[i].ID == r.Target.Panel {
						change(d.Document.Page.Panels[i].Layers)
					}
				}
				rel, e := filepath.Rel(w.Root, d.File)
				if e != nil {
					return e
				}
				edits = append(edits, DocumentEdit{File: filepath.ToSlash(rel), Document: d.Document})
			}
			if !found {
				return fmt.Errorf("character target layer not found")
			}
			local := *s
			local.storage = nil
			var result EditResult
			e := local.applyEdit(ctx, EditRequest{ProjectFile: w.Owner, ExpectedRevision: r.ExpectedRevision, Edits: edits}, w, &result)
			return e
		})
		return nil, e
	default:
		return nil, fmt.Errorf("unknown library action")
	}
}
