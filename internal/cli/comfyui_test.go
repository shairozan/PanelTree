package cli

import (
	"bytes"
	"context"
	"encoding/json"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestComfyRuntimeDiscovery(t *testing.T) {
	dir := t.TempDir()
	profile := `{"version":"comfy-profile/v1","name":"test","revision":"1","backend_identity":"comfy-test","models":{"checkpoint":"sha256:test"},"output_node":"out","workflow":{"out":{"class_type":"SaveImage","inputs":{}},"gen":{"class_type":"Fake","inputs":{"prompt":"","seed":0,"width":64,"height":64}}},"bindings":{"prompt":{"node":"gen","input":"prompt"},"seed":{"node":"gen","input":"seed"},"width":{"node":"gen","input":"width"},"height":{"node":"gen","input":"height"}}}`
	if e := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(profile), 0600); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(dir, "runtime.yaml")
	if e := os.WriteFile(cfg, []byte("comfyui-url: http://127.0.0.1:8188\ncomfyui-profile: profile.json\n"), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := executeProject(t, "renderers", "--config", cfg)
	if e != nil {
		t.Fatal(e)
	}
	var caps []app.RendererCapability
	if e = json.Unmarshal([]byte(out), &caps); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range caps {
		if c.Name == "comfyui" && c.Available {
			found = true
		}
	}
	if !found {
		t.Fatalf("configured ComfyUI unavailable: %s", out)
	}
	out, e = executeProject(t, "renderers")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(out), &caps); e != nil {
		t.Fatal(e)
	}
	for _, c := range caps {
		if c.Name == "comfyui" && c.Available {
			t.Fatal("configuration leaked across invocations")
		}
	}
}

func TestComfyCLIAndMCPWorkflow(t *testing.T) {
	var posts atomic.Int32
	var size image.Point
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/object_info":
			_, _ = w.Write([]byte(`{"Fake":{},"SaveImage":{}}`))
		case "/prompt":
			posts.Add(1)
			var b struct {
				Prompt map[string]struct {
					Inputs map[string]any `json:"inputs"`
				} `json:"prompt"`
			}
			if e := json.NewDecoder(r.Body).Decode(&b); e != nil {
				t.Error(e)
			}
			size = image.Pt(int(b.Prompt["gen"].Inputs["width"].(float64)), int(b.Prompt["gen"].Inputs["height"].(float64)))
			if prompt, ok := b.Prompt["gen"].Inputs["prompt"].(string); !ok || !strings.Contains(prompt, "Alex with short black hair") {
				t.Error("character description did not reach renderer")
			}
			_, _ = w.Write([]byte(`{"prompt_id":"id"}`))
		case "/history/id":
			_, _ = w.Write([]byte(`{"id":{"status":{"completed":true,"status_str":"success"},"outputs":{"out":{"images":[{"filename":"a.png","type":"output"}]}}}}`))
		case "/view":
			if e := png.Encode(w, image.NewNRGBA(image.Rectangle{Max: size})); e != nil {
				t.Error(e)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	profile := `{"version":"comfy-profile/v1","name":"test","revision":"1","backend_identity":"comfy-test","models":{"checkpoint":"sha256:test"},"output_node":"out","workflow":{"out":{"class_type":"SaveImage","inputs":{}},"gen":{"class_type":"Fake","inputs":{"prompt":"","seed":0,"width":64,"height":64}}},"bindings":{"prompt":{"node":"gen","input":"prompt"},"seed":{"node":"gen","input":"seed"},"width":{"node":"gen","input":"width"},"height":{"node":"gen","input":"height"}}}`
	if e := os.WriteFile(filepath.Join(root, "profile.json"), []byte(profile), 0600); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(root, "runtime.yaml")
	if e := os.WriteFile(cfg, []byte("comfyui-url: "+srv.URL+"\ncomfyui-profile: profile.json\n"), 0600); e != nil {
		t.Fatal(e)
	}
	book := filepath.Join(root, "book")
	if _, e := executeProject(t, "init", book); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(book, "project.yaml")
	pageFile := filepath.Join(book, "pages", "01.yaml")
	pageData, e := os.ReadFile(pageFile)
	if e != nil {
		t.Fatal(e)
	}
	pageData = bytes.ReplaceAll(pageData, []byte("path: ../assets/setting.png}"), []byte("path: ../assets/setting.png, character: {package: alex.json}}"))
	if e = os.WriteFile(pageFile, pageData, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(book, "alex.json"), []byte(`{"schema":"paneltree/character/v1","id":"alex","version":"1","description":"Alex with short black hair"}`), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := executeProject(t, "inspect", p)
	if e != nil {
		t.Fatal(e)
	}
	var view app.Inspection
	if e = json.Unmarshal([]byte(out), &view); e != nil {
		t.Fatal(e)
	}
	args := []string{"asset", "request", p, "--config", cfg, "--revision", string(view.Revision), "--key", "cli-generation", "--renderer", "comfyui", "--prompt", "moonlit city", "--seed", "17", "--page", "page-01", "--panel", "p1", "--layer", "setting", "--width", "120", "--height", "180"}
	out, e = executeProject(t, args...)
	if e != nil {
		t.Fatal(e)
	}
	var job app.Job
	if e = json.Unmarshal([]byte(out), &job); e != nil {
		t.Fatal(e)
	}
	if job.CharacterStatus != "current" || job.GenerationProvenance.Character == nil {
		t.Fatal("CLI omitted character provenance")
	}
	if _, e = executeProject(t, "jobs", "run", p, "--config", cfg); e != nil {
		t.Fatal(e)
	}
	if _, e = executeProject(t, "asset", "select", p, "--config", cfg, "--id", job.ID, "--revision", string(view.Revision)); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command(exe, "-test.run=^TestMCPProcess$")
	command.Env = append(os.Environ(), "PANELTREE_MCP_TEST_ROOT="+root, "PANELTREE_MCP_TEST_CONFIG="+cfg)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, e := protocol.NewClient(&protocol.Implementation{Name: "comfy-test", Version: "1"}, nil).Connect(ctx, &protocol.CommandTransport{Command: command}, nil)
	if e != nil {
		t.Fatalf("connect: %v %s", e, stderr.String())
	}
	defer func() { _ = c.Close() }()
	invoke := func(name string, in, out any) {
		t.Helper()
		r, e := c.CallTool(ctx, &protocol.CallToolParams{Name: name, Arguments: in})
		if e != nil || r.IsError {
			t.Fatalf("%s: %+v %v", name, r, e)
		}
		data, _ := json.Marshal(r.StructuredContent)
		var wrap struct{ Data json.RawMessage }
		if e = json.Unmarshal(data, &wrap); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(wrap.Data, out); e != nil {
			t.Fatal(e)
		}
	}
	invoke("project_open", map[string]any{"project_file": p}, &view)
	var capabilities []app.RendererCapability
	invoke("renderer_list", map[string]any{}, &capabilities)
	if !capabilities[1].Available || capabilities[1].OutputKinds[0] != "rgb" {
		t.Fatal("MCP omitted capability")
	}
	request := map[string]any{"project_file": p, "expected_revision": view.Revision, "key": "mcp-generation", "renderer": "comfyui", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "setting"}, "width": 120, "height": 180, "generation": map[string]any{"prompt": "moonlit city", "seed": 18}}
	invoke("asset_request", request, &job)
	invoke("asset_request", request, &job)
	invoke("jobs_run", map[string]any{"project_file": p, "workers": 1}, &[]app.Job{})
	invoke("jobs_status", map[string]any{"project_file": p, "id": job.ID}, &job)
	if job.State != "succeeded" || job.GenerationProvenance == nil || job.GenerationProvenance.Seed != 18 {
		t.Fatalf("MCP generation: %+v", job)
	}
	if job.CharacterStatus != "current" || job.GenerationProvenance.Character == nil || len(job.GenerationProvenance.Limitations) == 0 {
		t.Fatal("MCP omitted character provenance/capability limits")
	}
	invoke("asset_select", map[string]any{"project_file": p, "id": job.ID, "expected_revision": view.Revision}, &app.EditResult{})
	if posts.Load() != 2 {
		t.Fatalf("duplicate submissions: %d", posts.Load())
	}
}
