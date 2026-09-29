package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"path/filepath"
	"testing"
)

func TestJobCLIWorkflow(t *testing.T) {
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
	out, e = executeProject(t, "renderers")
	if e != nil {
		t.Fatal(e)
	}
	var caps []app.RendererCapability
	if e = json.Unmarshal([]byte(out), &caps); e != nil || len(caps) == 0 {
		t.Fatalf("capabilities %s %v", out, e)
	}
	out, e = executeProject(t, "asset", "request", p, "--revision", string(view.Revision), "--key", "cli-1", "--page", "page-01", "--panel", "p1", "--layer", "hero", "--width", "120", "--height", "180")
	if e != nil {
		t.Fatal(e)
	}
	var j app.Job
	if e = json.Unmarshal([]byte(out), &j); e != nil {
		t.Fatal(e)
	}
	if _, e = executeProject(t, "jobs", "run", p, "--workers", "2"); e != nil {
		t.Fatal(e)
	}
	out, e = executeProject(t, "jobs", "status", p, "--id", j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(out), &j); e != nil || j.State != "succeeded" {
		t.Fatalf("status %s %v", out, e)
	}
	if _, e = executeProject(t, "asset", "select", p, "--id", j.ID, "--revision", string(view.Revision)); e != nil {
		t.Fatal(e)
	}
	if _, e = executeProject(t, "jobs", "run", p, "--workers", "0"); e == nil {
		t.Fatal("invalid worker count accepted")
	}
}
