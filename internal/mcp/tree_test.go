package mcp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestTreeAllowsDisappearingRenderDirectory(t *testing.T) {
	root := t.TempDir()
	render := filepath.Join(root, ".render-completed")
	if e := os.Mkdir(render, 0700); e != nil {
		t.Fatal(e)
	}
	sibling := filepath.Join(root, "still-present.json")
	if e := os.WriteFile(sibling, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	removed, sawSibling := false, false
	err := checkTree(root, func(root string, visit fs.WalkDirFunc) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			result := visit(p, d, e)
			if e == nil && p == render {
				// Model the worker finishing after the scanner sees its directory entry
				// but before WalkDir opens that directory for its children.
				if e := os.Remove(render); e != nil {
					t.Fatal(e)
				}
				removed = true
			}
			if p == sibling {
				sawSibling = true
			}
			return result
		})
	})
	if err != nil {
		t.Fatalf("completed render broke tree preflight: %v", err)
	}
	if !removed || !sawSibling {
		t.Fatal("scan did not continue after render directory disappeared")
	}
}

func TestTreePreservesOtherFilesystemErrors(t *testing.T) {
	for _, cause := range []error{fs.ErrPermission, errors.New("storage failure")} {
		err := checkTree("root", func(root string, visit fs.WalkDirFunc) error {
			return visit(filepath.Join(root, "unreadable"), nil, &fs.PathError{Op: "open", Path: "unreadable", Err: cause})
		})
		if !errors.Is(err, cause) {
			t.Fatalf("lost filesystem failure %v: %v", cause, err)
		}
	}
}
