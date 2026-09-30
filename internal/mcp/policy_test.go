package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/workspace"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndirectPathsAndArtifactIntegrity(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	page := view.Documents[2].Document
	page.Page.Panels[0].Layers[0].Source.Path = filepath.Join(outside, "secret.png")
	r := call(t, c, "project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "edits": []app.DocumentEdit{{File: "pages/01.yaml", Document: page}}}, nil)
	if !r.IsError {
		t.Fatal("outside asset edit accepted")
	}
	target := app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}
	r = call(t, c, "project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "operations": []app.Operation{{Target: target, Action: "override", Artifact: filepath.Join(outside, "manual.png")}}}, nil)
	if !r.IsError {
		t.Fatal("outside manual override accepted")
	}
	var built buildOutput
	call(t, c, "build", map[string]any{"project_file": init.ProjectFile, "page": "page-01", "output": filepath.Join(root, "preview.png"), "width": 120, "height": 180}, &built)
	if e := os.WriteFile(built.Build.Output, []byte("modified"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := c.ReadResource(context.Background(), &protocol.ReadResourceParams{URI: built.Preview}); e == nil {
		t.Fatal("changed artifact silently served")
	}
	// Existing YAML references must be checked before loading the referenced file.
	original, e := os.ReadFile(init.ProjectFile)
	if e != nil {
		t.Fatal(e)
	}
	book := view.Documents[0].Document
	// Absolute paths are rejected at the boundary even before schema validation.
	book.Book.Chapters = []string{filepath.Join(outside, "secret.yaml")}
	b, e := yaml.Marshal(book)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(init.ProjectFile, b, 0600); e != nil {
		t.Fatal(e)
	}
	r = call(t, c, "project_validate", map[string]any{"project_file": init.ProjectFile}, nil)
	if !r.IsError {
		t.Fatal("outside document accepted")
	}
	if e = os.WriteFile(init.ProjectFile, original, 0600); e != nil {
		t.Fatal(e)
	}
	state, e := json.Marshal(map[string]app.LayerStatus{"page-01/p1/hero": {Manual: filepath.Join(outside, "manual.png")}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(init.ProjectFile), ".paneltree", "state.json"), state, 0600); e != nil {
		t.Fatal(e)
	}
	r = call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, nil)
	if !r.IsError {
		t.Fatal("outside persisted manual accepted")
	}
}

func TestCancellationAndPolling(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	var job app.Job
	call(t, c, "asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "key": "cancel", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}}, &job)
	call(t, c, "jobs_cancel", map[string]any{"project_file": init.ProjectFile, "id": job.ID}, &job)
	call(t, c, "jobs_status", map[string]any{"project_file": init.ProjectFile, "id": job.ID}, &job)
	if job.State != "cancelled" {
		t.Fatal(job)
	}
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- workspace.Open(context.Background(), init.ProjectFile, func(*workspace.Session) error { close(held); <-release; return nil })
	}()
	<-held
	defer func() {
		close(release)
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, e := c.CallTool(ctx, &protocol.CallToolParams{Name: "project_inspect", Arguments: map[string]any{"project_file": init.ProjectFile}})
	if e == nil {
		t.Fatal("blocked request ignored cancellation")
	}
	// A second request gets a response while the workspace remains locked: the
	// protocol connection survives cancellation and does not serialize unrelated tools.
	next, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	r, e := c.CallTool(next, &protocol.CallToolParams{Name: "renderer_list", Arguments: map[string]any{}})
	if e != nil || r.IsError {
		t.Fatalf("connection after cancellation: %+v %v", r, e)
	}
	r, e = c.CallTool(next, &protocol.CallToolParams{Name: "project_init", Arguments: map[string]any{"directory": filepath.Join(root, "after-cancel")}})
	if e != nil || r.IsError {
		t.Fatalf("cancelled request retained service gate: %+v %v", r, e)
	}
}

func TestBundleResources(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var out buildOutput
	if e := os.Mkdir(filepath.Join(root, "exports"), 0700); e != nil {
		t.Fatal(e)
	}
	call(t, c, "build", map[string]any{"project_file": init.ProjectFile, "page": "page-01", "bundle": filepath.Join(root, "exports"), "width": 120, "height": 180}, &out)
	found := false
	for _, uri := range out.Artifacts {
		r, e := c.ReadResource(context.Background(), &protocol.ReadResourceParams{URI: uri})
		if e != nil {
			t.Fatal(e)
		}
		if r.Contents[0].MIMEType == "image/svg+xml" {
			found = true
		}
	}
	if !found {
		t.Fatal("no editable SVG resource")
	}
}

func TestFontLicenseCannotEscapeRoots(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	fontDir := filepath.Join(root, "fonts")
	if e := os.Mkdir(fontDir, 0700); e != nil {
		t.Fatal(e)
	}
	font := filepath.Join(fontDir, "font.ttf")
	b, e := os.ReadFile(filepath.Join(root, "book", "assets", "Go-Regular.ttf"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(font, b, 0600); e != nil {
		t.Fatal(e)
	}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if e = os.WriteFile(secret, []byte("outside-root-secret"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(secret, font+".LICENSE"); e != nil {
		t.Skipf("symlink privilege unavailable: %v", e)
	}
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	page := view.Documents[2].Document
	page.Page.Panels[0].Layers[4].Source.Font = font
	data, e := yaml.Marshal(page)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(view.Documents[2].File, data, 0600); e != nil {
		t.Fatal(e)
	}
	r := call(t, c, "build", map[string]any{"project_file": init.ProjectFile, "page": "page-01", "bundle": root, "width": 120, "height": 180}, nil)
	if !r.IsError {
		t.Fatal("out-of-root font license was exported")
	}
}

func TestNestedProjectHasItsOwnOwner(t *testing.T) {
	parent := t.TempDir()
	if e := os.WriteFile(filepath.Join(parent, "project.yaml"), []byte("unrelated"), 0600); e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(parent, "allowed")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &app.Inspection{})
}

func TestRunningQueuePolling(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	var job app.Job
	for n := 0; n < 8; n++ {
		call(t, c, "asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "key": fmt.Sprintf("poll-%d", n), "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, "width": 1200, "height": 1800}, &job)
	}
	finished := make(chan error, 1)
	go func() {
		r, e := c.CallTool(context.Background(), &protocol.CallToolParams{Name: "jobs_run", Arguments: map[string]any{"project_file": init.ProjectFile, "workers": 2}})
		if e == nil && r.IsError {
			e = fmt.Errorf("runner failed: %v", r.StructuredContent)
		}
		finished <- e
	}()
	for {
		select {
		case e := <-finished:
			if e != nil {
				t.Fatal(e)
			}
			return
		default:
		}
		call(t, c, "jobs_status", map[string]any{"project_file": init.ProjectFile, "id": job.ID}, &job)
		call(t, c, "jobs_cancel", map[string]any{"project_file": init.ProjectFile, "id": job.ID}, &job)
	}
}
