// Package workspace serializes project transactions and recovers interrupted edits.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Edit struct {
	File     string         `json:"file"`
	Document model.Document `json:"document"`
}
type Changeset struct {
	ExpectedRevision model.Revision `json:"expected_revision"`
	Edits            []Edit         `json:"edits"`
}
type Session struct {
	Validate    func(*project.Snapshot) ([]byte, error)
	Root, Entry string
	Owner       string
	Snapshot    *project.Snapshot
	State       []byte
}

func Open(ctx context.Context, path string, fn func(*Session) error) error {
	entry, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	entry, err = filepath.EvalSymlinks(entry)
	if err != nil {
		return err
	}
	root := filepath.Dir(entry)
	// A nested page/chapter shares the owning project's lock and editorial state.
	for dir := root; ; dir = filepath.Dir(dir) {
		if _, e := os.Stat(filepath.Join(dir, "project.yaml")); e == nil {
			root = dir
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	meta := filepath.Join(root, ".paneltree")
	if err = os.MkdirAll(meta, 0700); err != nil {
		return err
	}
	if err = regularDirectory(meta); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(meta, "write.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		ok, e := tryLock(lock)
		if e != nil {
			return e
		}
		if ok {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer unlock(lock)
	if err = recoverTransaction(root); err != nil {
		return err
	}
	owner := entry
	if _, e := os.Stat(filepath.Join(root, "project.yaml")); e == nil {
		owner = filepath.Join(root, "project.yaml")
	}
	s := &Session{Root: root, Entry: entry, Owner: owner}
	if s.State, err = os.ReadFile(filepath.Join(meta, "state.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.Snapshot, err = project.Load(entry)
	if err != nil {
		return err
	}
	s.Snapshot.Revision = revision(s.Snapshot.Revision, s.State)
	return fn(s)
}
func revision(r model.Revision, state []byte) model.Revision {
	if len(state) == 0 {
		return r
	}
	return model.Revision(fmt.Sprintf("%x", sha256.Sum256(append([]byte(r), state...))))
}
func regularDirectory(p string) error {
	info, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace directory must not be a symlink: %s", p)
	}
	return nil
}

// SafePath excludes traversal and symlink aliases for all writable paths.
func SafePath(root, rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.Contains(rel, ":") {
		return "", fmt.Errorf("invalid relative path %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	p := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		info, e := os.Lstat(p)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink write rejected: %s", p)
		}
	}
	return p, nil
}

type journal struct {
	Files map[string][]byte `json:"files"`
}

func writeSync(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	return f.Close()
}
func recoverTransaction(root string) error {
	path := filepath.Join(root, ".paneltree", "pending.json")
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var j journal
	if e = json.Unmarshal(data, &j); e != nil {
		return fmt.Errorf("invalid transaction: %w", e)
	}
	// Validate every target before replaying any writes. Authoring documents may
	// use .yaml, .yml, or another filename accepted by the project loader.
	for rel, data := range j.Files {
		if _, e := SafePath(root, rel); e != nil {
			return e
		}
		clean := filepath.ToSlash(filepath.Clean(rel))
		if clean == ".paneltree/state.json" {
			if !json.Valid(data) {
				return fmt.Errorf("invalid transaction state")
			}
			continue
		}
		if strings.HasPrefix(clean, ".paneltree/") {
			return fmt.Errorf("invalid transaction target %s", rel)
		}
		var d model.Document
		if e := yaml.Unmarshal(data, &d); e != nil || d.Schema != model.Schema {
			return fmt.Errorf("invalid transaction document %s", rel)
		}
	}
	for rel, data := range j.Files {
		p, e := SafePath(root, rel)
		if e != nil {
			return e
		}
		if e = writeSync(p, data); e != nil {
			return e
		}
	}
	return os.Remove(path)
}

// Commit validates the complete staged graph before creating the recovery journal.
// Once journal publication succeeds, recovery always rolls forward to the new revision.
func (s *Session) Commit(ctx context.Context, c Changeset, state []byte) (*project.Snapshot, error) {
	if c.ExpectedRevision == "" || c.ExpectedRevision != s.Snapshot.Revision {
		return nil, fmt.Errorf("revision conflict: expected %s, current %s", c.ExpectedRevision, s.Snapshot.Revision)
	}
	stage, e := os.MkdirTemp(filepath.Join(s.Root, ".paneltree"), "edit-")
	if e != nil {
		return nil, e
	}
	defer func() { _ = os.RemoveAll(stage) }()
	known := map[string]bool{}
	j := journal{Files: map[string][]byte{}}
	for _, d := range s.Snapshot.Documents {
		rel, e := filepath.Rel(s.Root, d.File)
		if e != nil {
			return nil, e
		}
		if _, e = SafePath(s.Root, rel); e != nil {
			return nil, e
		}
		known[filepath.Clean(rel)] = true
		data, e := os.ReadFile(d.File)
		if e != nil {
			return nil, e
		}
		dest := filepath.Join(stage, rel)
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return nil, e
		}
		if e = writeSync(dest, data); e != nil {
			return nil, e
		}
	}
	seen := map[string]bool{}
	for _, edit := range c.Edits {
		rel := filepath.Clean(filepath.FromSlash(edit.File))
		if !known[rel] || seen[rel] {
			return nil, fmt.Errorf("edit requires a unique existing document: %s", edit.File)
		}
		seen[rel] = true
		data, e := yaml.Marshal(edit.Document)
		if e != nil {
			return nil, e
		}
		if e = writeSync(filepath.Join(stage, rel), data); e != nil {
			return nil, e
		}
		j.Files[rel] = data
	}
	entry, e := filepath.Rel(s.Root, s.Entry)
	if e != nil {
		return nil, e
	}
	next, e := project.Load(filepath.Join(stage, entry))
	if e != nil {
		return nil, e
	}
	for _, d := range next.Documents {
		rel, e := filepath.Rel(stage, d.File)
		if e != nil || !known[rel] {
			return nil, fmt.Errorf("references must remain within existing workspace documents")
		}
	}
	for i := range next.Pages {
		rel, err := filepath.Rel(stage, next.Pages[i].File)
		if err != nil {
			return nil, err
		}
		next.Pages[i].File = filepath.Join(s.Root, rel)
	}
	if s.Validate != nil {
		state, e = s.Validate(next)
		if e != nil {
			return nil, e
		}
	}
	if state != nil {
		j.Files[".paneltree/state.json"] = state
	} else {
		state = s.State
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if len(j.Files) > 0 {
		data, e := json.Marshal(j)
		if e != nil {
			return nil, e
		}
		tmp := filepath.Join(s.Root, ".paneltree", "pending.tmp")
		if e = writeSync(tmp, data); e != nil {
			return nil, e
		}
		if e = os.Rename(tmp, filepath.Join(s.Root, ".paneltree", "pending.json")); e != nil {
			return nil, e
		}
		if e = recoverTransaction(s.Root); e != nil {
			return nil, e
		}
	}
	next, e = project.Load(s.Entry)
	if e != nil {
		return nil, e
	}
	next.Revision = revision(next.Revision, state)
	s.Snapshot = next
	s.State = state
	return next, nil
}
