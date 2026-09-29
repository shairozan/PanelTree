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
