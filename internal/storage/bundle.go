package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
)

type document struct {
	Path  string
	Value model.Document
}
type bundle struct {
	Jobs      []portableJob
	Library   []LibraryVersion
	Documents []document
	Files     map[string][]byte
}

func capture(entry string) (bundle, error) {
	out := bundle{Files: map[string][]byte{}}
	root := filepath.Dir(entry)
	snap, e := project.Load(entry)
	if e != nil {
		return out, e
	}
	docs := map[string]bool{}
	for _, d := range snap.Documents {
		rel, e := filepath.Rel(root, d.File)
		if e != nil {
			return out, e
		}
		if _, e = workspace.SafePath(root, rel); e != nil {
			return out, e
		}
		rel = filepath.ToSlash(rel)
		portable := rel
		if len(out.Documents) == 0 {
			portable = "project.yaml"
		}
		out.Documents = append(out.Documents, document{portable, d.Document})
		docs[rel] = true
	}
	dependencies := map[string]bool{}
	for _, d := range out.Documents {
		paths, e := documentDependencies(root, d.Path, d.Value)
		if e != nil {
			return out, e
		}
		for _, path := range paths {
			dependencies[path] = true
		}
	}
	for path := range dependencies {
		if docs[path] {
			continue
		}
		{
			refs, e := characterDependencies(root, path)
			if e != nil {
				return out, e
			}
			for _, ref := range refs {
				dependencies[ref] = true
			}
		}
	}
	total := int64(0)
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle rejects symlink %s", rel)
		}
		if d.IsDir() {
			if rel == ".git" || rel == ".paneltree/cache" || rel == ".paneltree/jobs" || strings.HasPrefix(rel, ".paneltree/edit-") {
				return filepath.SkipDir
			}
			return nil
		}
		if docs[rel] {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(rel))
		keep := dependencies[rel] || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".svg" || ext == ".ttf" || ext == ".otf" || strings.HasSuffix(rel, ".LICENSE") || (strings.HasPrefix(rel, "characters/") && (ext == ".json" || ext == ".yaml" || ext == ".yml")) || (strings.HasPrefix(rel, ".paneltree/references/") || strings.HasPrefix(rel, ".paneltree/imports/")) || rel == ".paneltree/state.json"
		if !keep {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		total += info.Size()
		if !info.Mode().IsRegular() || info.Size() > 128<<20 || total > 1<<30 {
			return fmt.Errorf("bundle file/total size limit exceeded")
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out.Files[rel] = data
		return nil
	})
	if e == nil {
		for path := range dependencies {
			if !docs[path] {
				if _, ok := out.Files[path]; !ok {
					return out, fmt.Errorf("missing dependency %s", path)
				}
			}
		}
	}
	if e == nil {
		data, err := readPortableFile(root, ".paneltree/library-versions.json", 4<<20)
		if err == nil {
			e = json.Unmarshal(data, &out.Library)
		} else if !os.IsNotExist(err) {
			e = err
		}
	}
	if e == nil {
		e = captureManual(root, &out)
	}
	if e == nil {
		out.Jobs, e = captureJobs(root)
	}
	return out, e
}
func materialize(root string, b bundle) error {
	write := func(rel string, data []byte) error {
		p, e := workspace.SafePath(root, rel)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		return os.WriteFile(p, data, 0600)
	}
	for _, d := range b.Documents {
		if e := ValidateDocumentPaths(root, d.Path, d.Value); e != nil {
			return e
		}
		data, e := yaml.Marshal(d.Value)
		if e != nil {
			return e
		}
		if e = write(d.Path, data); e != nil {
			return e
		}
	}
	for path, data := range b.Files {
		if e := write(path, data); e != nil {
			return e
		}
	}
	if len(b.Library) > 0 {
		data, e := json.Marshal(b.Library)
		if e != nil {
			return e
		}
		if e = write(".paneltree/library-versions.json", data); e != nil {
			return e
		}
	}
	if e := writePortableJobs(root, b.Jobs); e != nil {
		return e
	}
	_, e := project.Load(filepath.Join(root, "project.yaml"))
	return e
}
func publishDirectory(destination string, b bundle) error {
	if _, e := os.Lstat(destination); !os.IsNotExist(e) {
		return fmt.Errorf("destination already exists or is inaccessible")
	}
	parent := filepath.Dir(destination)
	stage, e := os.MkdirTemp(parent, ".paneltree-import-")
	if e != nil {
		return e
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if e = materialize(stage, b); e != nil {
		return e
	}
	return os.Rename(stage, destination)
}
func (Filesystem) Import(ctx context.Context, dest, entry string) error {
	return workspace.Open(ctx, entry, func(w *workspace.Session) error {
		b, e := capture(w.Owner)
		if e != nil {
			return e
		}
		return publishDirectory(dest, b)
	})
}
func (f Filesystem) Export(ctx context.Context, source, dest string) error {
	return f.Import(ctx, dest, filepath.Join(source, "project.yaml"))
}
func digest(b []byte) string               { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func documentJSON(d model.Document) []byte { b, _ := json.Marshal(d); return b }
