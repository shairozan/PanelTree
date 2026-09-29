package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"path/filepath"
	"testing"
)

func TestEditorialCLI(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	dir := filepath.Join(t.TempDir(), "book")
	if _, e := executeProject(t, "init", dir); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, "project.yaml")
	out, e := executeProject(t, "inspect", p)
	if e != nil {
		t.Fatal(e)
	}
	var view app.Inspection
	if e = json.Unmarshal([]byte(out), &view); e != nil {
		t.Fatal(e)
	}
	out, e = executeProject(t, "lock", p, "--revision", string(view.Revision), "--page", "page-01", "--panel", "p1", "--layer", "hero")
	if e != nil {
		t.Fatal(e)
	}
	var result app.EditResult
	if e = json.Unmarshal([]byte(out), &result); e != nil {
		t.Fatal(e)
	}
	if result.Layers["page-01/p1/hero"].Lock != "all" {
		t.Fatal("plain lock did not lock all")
	}
	if _, e = executeProject(t, "unlock", p, "--revision", string(view.Revision), "--page", "page-01", "--panel", "p1", "--layer", "hero"); e == nil {
		t.Fatal("stale CLI unlock succeeded")
	}
	if _, e = executeProject(t, "unlock", p, "--revision", string(result.Revision), "--page", "page-01", "--panel", "p1", "--layer", "hero"); e != nil {
		t.Fatal(e)
	}
}
