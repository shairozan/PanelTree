package app

import (
	"bytes"
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
		apply := func(w *workspace.Session, v storage.LibraryVersion) error {
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
		}
		if strings.HasPrefix(r.ProjectFile, "pg:") {
			return nil, s.storage.BindLibrary(ctx, strings.TrimPrefix(r.ProjectFile, "pg:"), r.ID, r.Version, apply)
		}
		v, files, e := s.storage.LibraryFiles(ctx, r.ID, r.Version)
		if e != nil {
			return nil, e
		}
		e = s.openWorkspace(ctx, r.ProjectFile, func(w *workspace.Session) error {
			if r.ExpectedRevision == "" || r.ExpectedRevision != w.Snapshot.Revision {
				return fmt.Errorf("revision conflict")
			}
			// Only immutable additions are staged; never replace a project's local file.
			added := []string{}
			committed := false
			defer func() {
				if !committed {
					for _, path := range added {
						_ = os.Remove(path)
					}
				}
			}()
			for rel, data := range files {
				dest, e := workspace.SafePath(w.Root, rel)
				if e != nil {
					return e
				}
				prior, e := os.ReadFile(dest)
				if e == nil {
					if !bytes.Equal(prior, data) {
						return fmt.Errorf("library asset path collision")
					}
					continue
				}
				if !os.IsNotExist(e) {
					return e
				}
				if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
					return e
				}
				if e = os.WriteFile(dest, data, 0600); e != nil {
					return e
				}
				added = append(added, dest)
			}

			manifest, e := workspace.SafePath(w.Root, ".paneltree/library-versions.json")
			if e != nil {
				return e
			}
			prior, e := os.ReadFile(manifest)
			exists := e == nil
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			if len(prior) > 4<<20 {
				return fmt.Errorf("library manifest exceeds limit")
			}
			versions := []storage.LibraryVersion{}
			if exists {
				if e = json.Unmarshal(prior, &versions); e != nil {
					return e
				}
			}
			found := false
			for _, old := range versions {
				if old.ID == v.ID && old.Version == v.Version {
					found = true
				}
			}
			if !found {
				versions = append(versions, v)
			}
			data, e := json.Marshal(versions)
			if e != nil {
				return e
			}
			if e = replaceLibraryManifest(manifest, data); e != nil {
				return e
			}
			if e := apply(w, v); e != nil {
				if exists {
					if undo := replaceLibraryManifest(manifest, prior); undo != nil {
						return fmt.Errorf("%v; restoring manifest: %w", e, undo)
					}
				} else if undo := os.Remove(manifest); undo != nil {
					return fmt.Errorf("%v; restoring manifest: %w", e, undo)
				}
				return e
			}

			committed = true
			return nil
		})
		return nil, e
	default:
		return nil, fmt.Errorf("unknown library action")
	}
}

// LibraryCharacter returns published authoring metadata, never backend credentials.
type LibraryCharacterDetail struct {
	model.CharacterPackage
	PublishedCards map[string]string `json:"published_cards,omitempty"`
}

func (s *Service) LibraryCharacter(ctx context.Context, id, version string) (LibraryCharacterDetail, error) {
	var pkg LibraryCharacterDetail
	if s.storage == nil {
		return pkg, fmt.Errorf("PostgreSQL library is not configured")
	}
	v, files, e := s.storage.LibraryFiles(ctx, id, version)
	if e != nil {
		return pkg, e
	}
	if e = json.Unmarshal(files[v.Package], &pkg.CharacterPackage); e != nil {
		return pkg, e
	}
	if v.ReferenceSet != "" {
		var set ReferenceSet
		if e = json.Unmarshal(files[".paneltree/references/"+v.ReferenceSet+"/00000001.json"], &set); e != nil {
			return pkg, e
		}
		pub, ok := set.Published[v.ReferenceVersion]
		if !ok {
			return pkg, fmt.Errorf("published library references unavailable")
		}
		pkg.PublishedCards = pub.Cards
	}
	return pkg, nil
}
func (s *Service) LibraryMedia(ctx context.Context, id, version, path string) ([]byte, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("PostgreSQL library is not configured")
	}
	_, files, e := s.storage.LibraryFiles(ctx, id, version)
	if e != nil {
		return nil, e
	}
	data, ok := files[path]
	if !ok {
		return nil, fmt.Errorf("library image not found")
	}
	if e = validateArtwork(data); e != nil {
		return nil, e
	}
	return data, nil
}

func replaceLibraryManifest(path string, data []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".library-")
	if e != nil {
		return e
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
