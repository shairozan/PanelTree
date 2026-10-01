package mcp

import (
	"context"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/render"
	"path/filepath"
	"testing"
)

func TestIdeogramMCPProfileRequest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("IDEOGRAM_API_KEY", "fixture-key")
	cfg := render.GenerationConfig{Profiles: map[string]render.GenerationProfile{"story": {Renderer: "ideogram", Model: "ideogram-3", Operation: "generate", Speed: "quality", MagicPrompt: "off", StyleType: "auto"}}}
	service, e := app.NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	server, e := NewWithService([]string{root}, service)
	if e != nil {
		t.Fatal(e)
	}
	a, b := protocol.NewInMemoryTransports()
	ss, e := server.Connect(context.Background(), a, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = ss.Close() }()
	c, e := protocol.NewClient(&protocol.Implementation{Name: "ideogram-test", Version: "1"}, nil).Connect(context.Background(), b, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Close() }()
	var init app.InitResult
	call(t, c, "project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	var job app.Job
	call(t, c, "asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "key": "ideogram", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, "generation": map[string]any{"profile": "story", "prompt": "castle", "seed": 42}}, &job)
	if job.State != "queued" || job.GenerationProvenance == nil || job.GenerationProvenance.Profile != "story" {
		t.Fatalf("profile not passed: %+v", job)
	}
	// No network or paid generation is performed by submission.
}
