package storage

import (
	"fmt"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"path/filepath"
)

func documentDependencies(root, name string, d model.Document) ([]string, error) {
	var out []string
	add := func(base, path string) error {
		if path == "" {
			return nil
		}
		if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
			return fmt.Errorf("absolute project dependency rejected")
		}
		rel := filepath.ToSlash(filepath.Join(base, path))
		if _, e := workspace.SafePath(root, rel); e != nil {
			return e
		}
		out = append(out, rel)
		return nil
	}
	base := filepath.Dir(name)
	if d.Book != nil {
		for _, p := range d.Book.Chapters {
			if e := add(base, p); e != nil {
				return nil, e
			}
		}
	}
	if d.Chapter != nil {
		for _, p := range d.Chapter.Pages {
			if e := add(base, p); e != nil {
				return nil, e
			}
		}
	}
	var layers func([]model.Layer) error
	layers = func(items []model.Layer) error {
		for _, l := range items {
			if e := add(base, l.Mask); e != nil {
				return e
			}
			if l.Source != nil {
				if e := add(base, l.Source.Path); e != nil {
					return e
				}
				if e := add(base, l.Source.Font); e != nil {
					return e
				}
				if l.Source.Character != nil {
					if e := add("", l.Source.Character.Package); e != nil {
						return e
					}
				}
			}
			if e := layers(l.Children); e != nil {
				return e
			}
		}
		return nil
	}
	if d.Page != nil {
		for _, p := range d.Page.Panels {
			if e := layers(p.Layers); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
func ValidateDocumentPaths(root, name string, d model.Document) error {
	_, e := documentDependencies(root, name, d)
	return e
}
func characterDependencies(root, path string) ([]string, error) {
	data, e := readPortableFile(root, path, 128<<20)
	if e != nil {
		return nil, e
	}
	var pkg model.CharacterPackage
	if e = yaml.Unmarshal(data, &pkg); e != nil || pkg.Schema != "paneltree/character/v1" {
		return nil, nil
	}
	var out []string
	add := func(refs []model.CharacterReference) error {
		for _, r := range refs {
			if _, e := workspace.SafePath(root, r.Path); e != nil {
				return e
			}
			out = append(out, r.Path)
		}
		return nil
	}
	if e = add(pkg.References); e != nil {
		return nil, e
	}
	if e = add(pkg.Props); e != nil {
		return nil, e
	}
	for _, states := range [][]model.CharacterState{pkg.Costumes, pkg.Expressions, pkg.Poses} {
		for _, st := range states {
			if e = add(st.References); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
