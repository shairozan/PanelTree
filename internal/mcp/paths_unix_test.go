//go:build !windows

package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymlinkRootEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Fatal(e)
	}
	c := client(t, root)
	r := call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "escape", "book")}, nil)
	if !r.IsError {
		t.Fatal("symlink escape accepted")
	}
	if _, e := os.Stat(filepath.Join(outside, "book")); !os.IsNotExist(e) {
		t.Fatal("outside write occurred")
	}
}
