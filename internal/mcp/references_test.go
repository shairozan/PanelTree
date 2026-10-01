package mcp

import (
	"github.com/shairozan/PanelTree/app"
	"path/filepath"
	"testing"
)

func TestReferenceMCP(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var set app.ReferenceSet
	call(t, c, "character_reference", app.ReferenceRequest{ProjectFile: init.ProjectFile, Action: "create", Set: "hero"}, &set)
	if set.Revision == "" {
		t.Fatal("missing reference result")
	}
}
