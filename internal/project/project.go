// Package project loads and validates declarative projects without rendering.
package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type LoadedPage struct {
	File string     `json:"file"`
	Page model.Page `json:"page"`
}
type LoadedDocument struct {
	File     string         `json:"file"`
	Document model.Document `json:"document"`
}

// Snapshot preserves author-declared order and file locations for relative assets.
type Snapshot struct {
	Revision  model.Revision   `json:"revision"`
	Documents []LoadedDocument `json:"documents"`
	Pages     []LoadedPage     `json:"pages"`
}
type loader struct {
	result       Snapshot
	active, seen map[string]bool
	ids          map[model.ID]bool
}

func Load(path string) (*Snapshot, error) {
	l := loader{active: map[string]bool{}, seen: map[string]bool{}, ids: map[model.ID]bool{}}
	if err := l.visit(path, "", 0); err != nil {
		return nil, err
	}
	docs := make([]model.Document, 0, len(l.result.Documents))
	for _, d := range l.result.Documents {
		docs = append(docs, d.Document)
	}
	data, err := yaml.Marshal(docs)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	l.result.Revision = model.Revision(hex.EncodeToString(sum[:]))
	return &l.result, nil
}
func (l *loader) visit(path, expected string, depth int) error {
	if depth > 64 || len(l.seen) >= 1024 {
		return fmt.Errorf("%s:1:1: project reference limit exceeded", path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("%s:1:1: %w", path, err)
	}
	key := absolute
	if filepath.Separator == '\\' {
		key = strings.ToLower(key)
	}
	if l.active[key] {
		return fmt.Errorf("%s:1:1: reference cycle", path)
	}
	if l.seen[key] {
		return fmt.Errorf("%s:1:1: duplicate document reference", path)
	}
	l.active[key] = true
	l.seen[key] = true
	defer delete(l.active, key)
	doc, root, err := read(absolute)
	if err != nil {
		return err
	}
	kind, id, refs := documentInfo(doc)
	if kind == "" {
		return diagnostic(absolute, root, "exactly one book, chapter or page is required")
	}
	if expected != "" && kind != expected {
		return diagnostic(absolute, root, "expected %s document, got %s", expected, kind)
	}
	if doc.Schema != model.Schema {
		return diagnostic(absolute, field(root, "schema"), "unsupported schema %q", doc.Schema)
	}
	body := field(root, kind)
	if !validID(id) {
		return diagnostic(absolute, field(body, "id"), "invalid id %q", id)
	}
	if l.ids[id] {
		return diagnostic(absolute, field(body, "id"), "duplicate document id %q", id)
	}
	l.ids[id] = true
	l.result.Documents = append(l.result.Documents, LoadedDocument{absolute, doc})
	if kind == "page" {
		if err := validatePage(absolute, *doc.Page, body); err != nil {
			return err
		}
		l.result.Pages = append(l.result.Pages, LoadedPage{absolute, *doc.Page})
		return nil
	}
	if len(refs) == 0 {
		return diagnostic(absolute, body, "%s requires at least one child reference", kind)
	}
	next, refField := "page", "pages"
	if kind == "book" {
		next, refField = "chapter", "chapters"
	}
	for i, ref := range refs {
		n := field(body, refField).Content[i]
		if ref == "" || filepath.IsAbs(ref) || filepath.VolumeName(ref) != "" {
			return diagnostic(absolute, n, "reference must be a nonempty relative path")
		}
		if err := l.visit(filepath.Join(filepath.Dir(absolute), filepath.FromSlash(ref)), next, depth+1); err != nil {
			return diagnostic(absolute, n, "reference %q: %v", ref, err)
		}
	}
	return nil
}
func documentInfo(d model.Document) (string, model.ID, []string) {
	count := 0
	kind := ""
	var id model.ID
	var refs []string
	if d.Book != nil {
		count++
		kind = "book"
		id = d.Book.ID
		refs = d.Book.Chapters
	}
	if d.Chapter != nil {
		count++
		kind = "chapter"
		id = d.Chapter.ID
		refs = d.Chapter.Pages
	}
	if d.Page != nil {
		count++
		kind = "page"
		id = d.Page.ID
	}
	if count != 1 {
		return "", "", nil
	}
	return kind, id, refs
}
func read(path string) (model.Document, *yaml.Node, error) {
	var doc model.Document
	f, err := os.Open(path)
	if err != nil {
		return doc, nil, fmt.Errorf("%s:1:1: %w", path, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	closeErr := f.Close()
	if readErr != nil {
		return doc, nil, fmt.Errorf("%s:1:1: %w", path, readErr)
	}
	if closeErr != nil {
		return doc, nil, closeErr
	}
	if len(data) > 1024*1024 {
		return doc, nil, fmt.Errorf("%s:1:1: YAML exceeds 1 MiB", path)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		return doc, nil, fmt.Errorf("%s:1:1: %w", path, err)
	}
	if len(root.Content) != 1 {
		return doc, nil, fmt.Errorf("%s:1:1: empty YAML document", path)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return doc, nil, fmt.Errorf("%s:1:1: expected one YAML document", path)
	}
	node := root.Content[0]
	if err := strict(path, node, reflect.TypeFor[model.Document](), 0); err != nil {
		return doc, node, err
	}
	if err := node.Decode(&doc); err != nil {
		return doc, node, diagnostic(path, node, "%v", err)
	}
	return doc, node, nil
}
func strict(path string, n *yaml.Node, t reflect.Type, depth int) error {
	if depth > 64 {
		return diagnostic(path, n, "YAML nesting exceeds 64")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return diagnostic(path, n, "YAML aliases and anchors are unsupported")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if n.Tag == "!!null" {
		return diagnostic(path, n, "explicit null is unsupported; omit optional fields")
	}
	switch t.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return diagnostic(path, n, "expected mapping")
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fields[strings.Split(f.Tag.Get("yaml"), ",")[0]] = f.Type
		}
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return diagnostic(path, k, "mapping keys must be strings")
			}
			if seen[k.Value] {
				return diagnostic(path, k, "duplicate key %q", k.Value)
			}
			seen[k.Value] = true
			ft, ok := fields[k.Value]
			if !ok {
				return diagnostic(path, k, "unknown field %q", k.Value)
			}
			if err := strict(path, v, ft, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode {
			return diagnostic(path, n, "expected sequence")
		}
		for _, v := range n.Content {
			if err := strict(path, v, t.Elem(), depth+1); err != nil {
				return err
			}
		}
	default:
		if n.Kind != yaml.ScalarNode {
			return diagnostic(path, n, "expected scalar")
		}
		if t.Kind() == reflect.String && n.Tag != "!!str" {
			return diagnostic(path, n, "expected string (quote numeric IDs)")
		}
	}
	return nil
}
func field(n *yaml.Node, name string) *yaml.Node {
	if n != nil {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == name {
				return n.Content[i+1]
			}
		}
	}
	return n
}
func diagnostic(path string, n *yaml.Node, format string, args ...any) error {
	line, col := 1, 1
	if n != nil {
		line, col = n.Line, n.Column
	}
	return fmt.Errorf("%s:%d:%d: %s", path, line, col, fmt.Sprintf(format, args...))
}
