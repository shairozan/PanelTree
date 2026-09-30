package project

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzStrictDocument(f *testing.F) {
	f.Add([]byte("schema: paneltree/v0.1\npage:\n  id: p\n  canvas: {width: 100, height: 100}\n  layout: {panel: a}\n  panels: [{id: a, layers: []}]\n"))
	f.Add([]byte("schema: paneltree/v0.1\npage: &p {id: p}\nbook: *p\n"))
	f.Add([]byte("page: {id: a, id: b}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "input.yaml")
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
		doc, node, e := read(path)
		if e != nil {
			return
		}
		// Exercise shape and page validation without following fuzzed filesystem references.
		kind, _, _ := documentInfo(doc)
		if kind == "page" {
			_ = validatePage(path, *doc.Page, field(node, "page"))
		}
	})
}
