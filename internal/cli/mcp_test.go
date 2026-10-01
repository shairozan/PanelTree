package cli

import (
	"bytes"
	"context"
	"encoding/json"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type observedMCPInput struct {
	*io.PipeReader
	started   chan struct{}
	once      sync.Once
	closed    chan struct{}
	closeOnce sync.Once
}

func (r *observedMCPInput) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.PipeReader.Read(p)
}

func (r *observedMCPInput) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return r.PipeReader.Close()
}
func TestMCPServeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	cmd := Command()
	input := &observedMCPInput{PipeReader: reader, started: make(chan struct{}), closed: make(chan struct{})}
	cmd.SetIn(input)
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"mcp", "serve", "--root", t.TempDir()})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	select {
	case <-input.started:
	case err := <-done:
		t.Fatalf("MCP exited before reading: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("MCP did not start reading")
	}
	cancel()
	select {
	case <-done:
		select {
		case <-input.closed:
		default:
			t.Fatal("input not closed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("MCP shutdown did not close blocked input")
	}
}

func TestMCPProcess(t *testing.T) {
	root := os.Getenv("PANELTREE_MCP_TEST_ROOT")
	if root == "" {
		t.Skip("stdio subprocess helper")
	}
	cmd := Command()
	args := []string{"mcp", "serve", "--root", root}
	if cfg := os.Getenv("PANELTREE_MCP_TEST_CONFIG"); cfg != "" {
		args = append(args, "--config", cfg)
	}
	cmd.SetArgs(args)
	if e := cmd.Execute(); e != nil {
		_, _ = os.Stderr.WriteString(e.Error())
		os.Exit(1)
	}
	os.Exit(0)
}
func TestMCPStdioAndCLIEquivalence(t *testing.T) {
	root := t.TempDir()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command(exe, "-test.run=^TestMCPProcess$")
	command.Env = append(os.Environ(), "PANELTREE_MCP_TEST_ROOT="+root)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := protocol.NewClient(&protocol.Implementation{Name: "real-stdio-client", Version: "1"}, nil).Connect(ctx, &protocol.CommandTransport{Command: command}, nil)
	if e != nil {
		t.Fatalf("initialize MCP: %v; stderr: %s", e, stderr.String())
	}
	defer func() { _ = c.Close() }()
	invoke := func(name string, args any, out any) {
		t.Helper()
		r, e := c.CallTool(ctx, &protocol.CallToolParams{Name: name, Arguments: args})
		if e != nil || r.IsError {
			t.Fatalf("%s: %+v %v", name, r, e)
		}
		b, _ := json.Marshal(r.StructuredContent)
		var w struct{ Data json.RawMessage }
		if e = json.Unmarshal(b, &w); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(w.Data, out); e != nil {
			t.Fatal(e)
		}
	}
	var init app.InitResult
	invoke("project_init", map[string]any{"directory": filepath.Join(root, "book")}, &init)
	var view app.Inspection
	invoke("project_open", map[string]any{"project_file": init.ProjectFile}, &view)
	doc := view.Documents[2]
	doc.Document.Page.Panels[0].Layers[0].Role = "mcp-edited"
	var edited app.EditResult
	invoke("project_apply_changes", map[string]any{"project_file": init.ProjectFile, "expected_revision": view.Revision, "edits": []app.DocumentEdit{{File: "pages/01.yaml", Document: doc.Document}}}, &edited)
	var j app.Job
	invoke("asset_request", map[string]any{"project_file": init.ProjectFile, "expected_revision": edited.Revision, "key": "stdio-asset", "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, "width": 120, "height": 180}, &j)
	invoke("jobs_run", map[string]any{"project_file": init.ProjectFile, "workers": 1}, &[]app.Job{})
	invoke("jobs_status", map[string]any{"project_file": init.ProjectFile, "id": j.ID}, &j)
	if j.State != "succeeded" {
		t.Fatal(j)
	}
	var built struct {
		Build   app.BuildResult
		Preview string
	}
	invoke("build", map[string]any{"project_file": init.ProjectFile, "page": "page-01", "output": filepath.Join(root, "mcp.png"), "width": 120, "height": 180}, &built)
	preview, e := c.ReadResource(ctx, &protocol.ReadResourceParams{URI: built.Preview})
	if e != nil {
		t.Fatal(e)
	}
	cliPath := filepath.Join(root, "cli.png")
	out, e := executeProject(t, "build", init.ProjectFile, "--page", "page-01", "--output", cliPath, "--width", "120", "--height", "180")
	if e != nil {
		t.Fatal(e)
	}
	var cli app.BuildResult
	if e = json.Unmarshal([]byte(out), &cli); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(cliPath)
	if e != nil {
		t.Fatal(e)
	}
	if cli.Revision != built.Build.Revision || !bytes.Equal(b, preview.Contents[0].Blob) {
		t.Fatal("MCP/CLI output differs")
	}
}
func TestMCPRequiresRoot(t *testing.T) {
	out, e := executeProject(t, "mcp", "serve")
	if e == nil || out != "" {
		t.Fatalf("expected clean stdout and missing roots error: %q %v", out, e)
	}
	if e.Error() != "at least one MCP root is required" {
		t.Fatal(e)
	}
}
