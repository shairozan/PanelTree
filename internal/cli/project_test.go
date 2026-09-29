package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func executeProject(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := Command()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
func TestProjectWorkflow(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	dest := filepath.Join(t.TempDir(), "book")
	if _, err := executeProject(t, "init", dest); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dest, "project.yaml")
	if _, err := executeProject(t, "validate", file); err != nil {
		t.Fatal(err)
	}
	output, err := executeProject(t, "inspect", file)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Revision string
		Pages    []struct{ Page struct{ ID string } }
	}
	if err := json.Unmarshal([]byte(output), &view); err != nil {
		t.Fatal(err)
	}
	if view.Revision == "" || len(view.Pages) != 2 || view.Pages[0].Page.ID != "page-01" || view.Pages[1].Page.ID != "page-02" {
		t.Fatalf("unexpected inspection: %s", output)
	}
}
func TestChildCommandConfigurationAndHelp(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	for _, verb := range []string{"init", "validate", "inspect"} {
		if _, err := executeProject(t, verb, "--help", "--config", filepath.Join(t.TempDir(), "missing.yaml")); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "must-not-exist")
	if _, err := executeProject(t, "init", dir, "--log-level", "invalid"); err == nil {
		t.Fatal("invalid config accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("init ran before validation: %v", err)
	}
}

func TestInspectResolvedBoundsAndOutputFit(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	dest := filepath.Join(t.TempDir(), "book")
	if _, err := executeProject(t, "init", dest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dest, "project.yaml")
	output, err := executeProject(t, "inspect", path)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Scenes []struct {
			Scene struct {
				Root   struct{ Children []json.RawMessage }
				Output struct{ Width, Height int }
			}
		}
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Scenes) != 2 {
		t.Fatalf("inspect omitted resolved scenes: %s", output)
	}
	if result.Scenes[0].Scene.Output.Width != 1200 || len(result.Scenes[0].Scene.Root.Children) == 0 {
		t.Fatal("missing output or geometry")
	}
	if _, err := executeProject(t, "inspect", path, "--width", "1080", "--height", "1920"); err == nil {
		t.Fatal("implicit aspect change accepted")
	}
	if _, err := executeProject(t, "inspect", path, "--width", "1080", "--height", "1920", "--fit", "contain"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCommand(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	dir := filepath.Join(t.TempDir(), "book")
	if _, err := executeProject(t, "init", dir); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "demo.png")
	if _, err := executeProject(t, "build", filepath.Join(dir, "project.yaml"), "--page", "page-01", "--output", out, "--width", "120", "--height", "180"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestBuildBundleCommand(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	dir := t.TempDir()
	book := filepath.Join(dir, "book")
	if _, err := executeProject(t, "init", book); err != nil {
		t.Fatal(err)
	}
	output, err := executeProject(t, "build", filepath.Join(book, "project.yaml"), "--page", "page-01", "--bundle", dir, "--width", "120", "--height", "180")
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Bundle string }
	if err = json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(result.Bundle, "page.svg")); err != nil {
		t.Fatal(err)
	}
}
