// Package acceptance_test exercises the shipped executable, not a command mock.
package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
)

func TestMVPExecutableWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := t.TempDir()
	binary := filepath.Join(root, "paneltree")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	compile := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "../cmd/paneltree")
	if out, e := compile.CombinedOutput(); e != nil {
		t.Fatalf("fresh executable build: %v\n%s", e, out)
	}
	cli := func(out any, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		b, e := cmd.Output()
		if e != nil {
			t.Fatalf("%v: %v\n%s", args, e, stderr.String())
		}
		if out != nil {
			if e = json.Unmarshal(b, out); e != nil {
				t.Fatalf("CLI JSON: %v: %s", e, b)
			}
		}
	}
	book := filepath.Join(root, "book")
	var initialized app.InitResult
	cli(&initialized, "init", book)
	project := initialized.ProjectFile
	var validated app.ValidateResult
	cli(&validated, "validate", project)
	if !validated.Valid || validated.PageCount != 2 {
		t.Fatalf("two-page demo: %+v", validated)
	}
	originals := map[string][]byte{}
	for _, rel := range []string{"assets/hero.png", "assets/Go-Regular.ttf", "assets/Go-Regular.ttf.LICENSE", "assets/LICENSE.txt", "pages/01.yaml", "pages/02.yaml"} {
		b, e := os.ReadFile(filepath.Join(book, rel))
		if e != nil {
			t.Fatal(e)
		}
		originals[rel] = b
	}
	for _, page := range []string{"page-01", "page-02"} {
		var cold, warm app.BuildResult
		cli(&cold, "build", project, "--page", page, "--bundle", book, "--width", "240", "--height", "360")
		cli(&warm, "build", project, "--page", page, "--output", filepath.Join(book, page+"-warm.png"), "--width", "240", "--height", "360")
		if warm.LeafRenders != 0 || warm.Recompositions != 0 || warm.BuildID != cold.BuildID {
			t.Fatalf("warm build: %+v", warm)
		}
		a := checkImage(t, cold.Output, page)
		b := checkImage(t, warm.Output, page)
		if !bytes.Equal(a, b) {
			t.Fatal("warm export differs")
		}
		for _, name := range []string{"page.svg", "page.yaml", "composition.json"} {
			if _, e := os.Stat(filepath.Join(cold.Bundle, name)); e != nil {
				t.Fatal(e)
			}
		}
		var relocated app.BuildResult
		cli(&relocated, "build", filepath.Join(cold.Bundle, "page.yaml"), "--output", filepath.Join(book, page+"-portable.png"), "--width", "240", "--height", "360")
		if !bytes.Equal(a, checkImage(t, relocated.Output, page)) {
			t.Fatal("portable bundle pixels differ")
		}
	}
	// Connect a real SDK client to the compiled binary over stdio. No model,
	// browser, AI service, network renderer or persistent background process.
	command := exec.CommandContext(ctx, binary, "mcp", "serve", "--root", root)
	command.Stderr = os.Stderr
	c, e := protocol.NewClient(&protocol.Implementation{Name: "mvp-acceptance", Version: "1"}, nil).Connect(ctx, &protocol.CommandTransport{Command: command}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Close() }()
	invoke := func(name string, args any, out any) *protocol.CallToolResult {
		t.Helper()
		r, err := c.CallTool(ctx, &protocol.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out != nil {
			if r.IsError {
				t.Fatalf("%s: %+v", name, r)
			}
			b, err := json.Marshal(r.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var wrapped struct{ Data json.RawMessage }
			if err = json.Unmarshal(b, &wrapped); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(wrapped.Data, out); err != nil {
				t.Fatal(err)
			}
		}
		return r
	}
	var second app.InitResult
	invoke("project_init", map[string]any{"directory": filepath.Join(root, "agent-book")}, &second)
	var view app.Inspection
	invoke("project_open", map[string]any{"project_file": project}, &view)
	target := app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}
	var job app.Job
	invoke("asset_request", map[string]any{"project_file": project, "expected_revision": view.Revision, "key": "acceptance-hero", "target": target, "width": 240, "height": 360}, &job)
	cli(nil, "jobs", "run", project, "--workers", "2")
	invoke("jobs_status", map[string]any{"project_file": project, "id": job.ID}, &job)
	if job.State != "succeeded" {
		t.Fatalf("job: %+v", job)
	}
	var edit app.EditResult
	invoke("asset_select", map[string]any{"project_file": project, "expected_revision": view.Revision, "id": job.ID}, &edit)
	stale := invoke("project_apply_changes", map[string]any{"project_file": project, "expected_revision": view.Revision, "operations": []app.Operation{{Target: target, Action: "review"}}}, nil)
	if !stale.IsError {
		t.Fatal("MCP accepted stale revision")
	}
	invoke("project_apply_changes", map[string]any{"project_file": project, "expected_revision": edit.Revision, "operations": []app.Operation{{Target: target, Action: "review"}}}, &edit)
	cli(&edit, "approve", project, "--revision", string(edit.Revision), "--page", "page-01", "--panel", "p1", "--layer", "hero", "--artifact", "assets/hero.png")
	invoke("project_apply_changes", map[string]any{"project_file": project, "expected_revision": edit.Revision, "operations": []app.Operation{{Target: target, Action: "lock", Scope: "all"}}}, &edit)
	blocked := invoke("project_apply_changes", map[string]any{"project_file": project, "expected_revision": edit.Revision, "operations": []app.Operation{{Target: target, Action: "clear-selection"}}}, nil)
	if !blocked.IsError {
		t.Fatal("MCP bypassed all-lock")
	}
	var built struct {
		Build     app.BuildResult
		Preview   string
		Artifacts []string
	}
	invoke("build", map[string]any{"project_file": project, "page": "page-01", "bundle": book, "width": 240, "height": 360}, &built)
	preview, e := c.ReadResource(ctx, &protocol.ReadResourceParams{URI: built.Preview})
	if e != nil {
		t.Fatal(e)
	}
	if len(preview.Contents) != 1 || preview.Contents[0].MIMEType != "image/png" || !bytes.Equal(preview.Contents[0].Blob, checkImage(t, built.Build.Output, "page-01")) {
		t.Fatal("MCP preview differs from export")
	}
	foundSVG := false
	for _, uri := range built.Artifacts {
		r, err := c.ReadResource(ctx, &protocol.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range r.Contents {
			if part.MIMEType == "image/svg+xml" {
				foundSVG = true
			}
		}
	}
	if !foundSVG {
		t.Fatal("MCP bundle has no editable SVG resource")
	}
	var queued app.Job
	invoke("asset_request", map[string]any{"project_file": project, "expected_revision": edit.Revision, "key": "cancel-this", "target": app.LayerTarget{Page: "page-02", Panel: "p1", Layer: "hero"}}, &queued)
	invoke("jobs_cancel", map[string]any{"project_file": project, "id": queued.ID}, &queued)
	cli(&queued, "jobs", "status", project, "--id", queued.ID)
	if queued.State != "cancelled" {
		t.Fatalf("durable cancellation: %+v", queued)
	}
	for rel, before := range originals {
		after, err := os.ReadFile(filepath.Join(book, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("workflow overwrote authored input %s", rel)
		}
	}
}

// Semantic image assertions avoid platform-sensitive golden PNG compression.
func checkImage(t *testing.T, path, page string) []byte {
	t.Helper()
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	im, e := png.Decode(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	if im.Bounds().Dx() != 240 || im.Bounds().Dy() != 360 {
		t.Fatalf("wrong demo bounds: %v", im.Bounds())
	}
	colors := map[[4]uint32]bool{}
	opaque, transparent := 0, 0
	for y := 0; y < 360; y++ {
		for x := 0; x < 240; x++ {
			r, g, b, a := im.At(x, y).RGBA()
			colors[[4]uint32{r, g, b, a}] = true
			if a == 65535 {
				opaque++
			}
			if a == 0 {
				transparent++
			}
		}
	}
	if len(colors) < 4 || opaque == 0 {
		t.Fatalf("blank or degenerate demo: colors=%d opaque=%d", len(colors), opaque)
	}
	if page == "page-01" && transparent != 0 {
		t.Fatal("white page backdrop has holes")
	}
	if page == "page-02" && transparent == 0 {
		t.Fatal("character cutout lost transparency")
	}
	return data
}
