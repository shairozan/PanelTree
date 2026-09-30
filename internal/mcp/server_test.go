package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"os"
	"path/filepath"
	"testing"
)

func client(t *testing.T, root string) *protocol.ClientSession {
	t.Helper()
	s, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	a, b := protocol.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	c, err := protocol.NewClient(&protocol.Implementation{Name: "sprint08-test", Version: "1"}, nil).Connect(context.Background(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func call(t *testing.T, c *protocol.ClientSession, name string, in any, out any) *protocol.CallToolResult {
	t.Helper()
	r, e := c.CallTool(context.Background(), &protocol.CallToolParams{Name: name, Arguments: in})
	if e != nil {
		t.Fatal(e)
	}
	if out != nil {
		if r.IsError {
			b, _ := json.Marshal(r)
			t.Fatalf("%s: %s", name, b)
		}
		b, e := json.Marshal(r.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		var wrap struct {
			Data json.RawMessage `json:"data"`
		}
		if e = json.Unmarshal(b, &wrap); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(wrap.Data, out); e != nil {
			t.Fatalf("decode %s: %s: %v", name, b, e)
		}
	}
	return r
}
func TestClientAuthoringWorkflow(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	book := filepath.Join(root, "book")
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": book}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	if view.Revision == "" || len(view.Documents) == 0 {
		t.Fatal("missing project snapshot")
	}
	doc := view.Documents[0].Document
	doc.Book.Title = "MCP title"
	var edit app.EditResult
	call(t, c, "project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "edits": []app.DocumentEdit{{File: "project.yaml", Document: doc}}}, &edit)
	var job app.Job
	call(t, c, "asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": edit.Revision, "key": "hero-1", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, "width": 120, "height": 180}, &job)
	call(t, c, "jobs_run", map[string]any{"project_file": init.ProjectFile, "workers": 2}, &[]app.Job{})
	call(t, c, "jobs_status", map[string]any{"project_file": init.ProjectFile, "id": job.ID}, &job)
	if job.State != "succeeded" {
		t.Fatalf("job %+v", job)
	}
	var built struct {
		Build   app.BuildResult `json:"build"`
		Preview string          `json:"preview"`
	}
	call(t, c, "build", map[string]any{"project_file": init.ProjectFile, "page": "page-01", "output": filepath.Join(book, "mcp.png"), "width": 120, "height": 180}, &built)
	preview, e := c.ReadResource(context.Background(), &protocol.ReadResourceParams{URI: built.Preview})
	if e != nil {
		t.Fatal(e)
	}
	if len(preview.Contents) != 1 || preview.Contents[0].MIMEType != "image/png" || len(preview.Contents[0].Blob) == 0 {
		t.Fatal("no PNG resource")
	}
	cli, e := app.NewService().Build(context.Background(), app.BuildRequest{ProjectFile: init.ProjectFile, PageID: "page-01", Output: filepath.Join(book, "shared.png"), Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(cli.Output)
	if e != nil {
		t.Fatal(e)
	}
	if cli.Revision != built.Build.Revision || !bytes.Equal(b, preview.Contents[0].Blob) {
		t.Fatal("shared service content differs")
	}
	resources, e := c.ListResources(context.Background(), nil)
	if e != nil || len(resources.Resources) < 2 {
		t.Fatalf("resources: %v %v", resources, e)
	}
	tools, e := c.ListTools(context.Background(), nil)
	if e != nil || len(tools.Tools) < 14 {
		t.Fatalf("tools: %v %v", tools, e)
	}
}

func TestPolicyErrors(t *testing.T) {
	root := t.TempDir()
	c := client(t, root)
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	call(t, c, "project_inspect", map[string]any{"project_file": init.ProjectFile}, &view)
	for _, tc := range []struct {
		name string
		args any
	}{
		{"project_init", map[string]any{"directory": filepath.Join(t.TempDir(), "escape")}},
		{"project_inspect", map[string]any{"project_file": filepath.Join(t.TempDir(), "project.yaml")}},
		{"project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": "stale"}},
		{"asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "key": "no-generator", "renderer": "comfyui", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}}},
		{"build", map[string]any{"project_file": init.ProjectFile, "output": filepath.Join(t.TempDir(), "outside.png"), "page": "page-01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, c, tc.name, tc.args, nil)
			if !r.IsError || r.StructuredContent == nil {
				t.Fatalf("missing typed error: %+v", r)
			}
		})
	}
	var locked app.EditResult
	target := app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}
	call(t, c, "project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "operations": []app.Operation{{Action: "lock", Scope: "asset", Target: target}}}, &locked)
	r := call(t, c, "project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": locked.Revision, "operations": []app.Operation{{Action: "clear-selection", Target: target}}}, nil)
	if !r.IsError {
		t.Fatal("lock bypass accepted")
	}
	if _, e := c.ReadResource(context.Background(), &protocol.ReadResourceParams{URI: "file:///etc/passwd"}); e == nil {
		t.Fatal("arbitrary resource accepted")
	}
}
