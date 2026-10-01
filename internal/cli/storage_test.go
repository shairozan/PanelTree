package cli

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresCLIProject(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	t.Setenv("PANELTREE_DATABASE_URL", dsn)
	root := t.TempDir()
	config := filepath.Join(root, "runtime.yaml")
	if e := os.WriteFile(config, []byte("storage:\n  dsn-env: PANELTREE_DATABASE_URL\n  blob-root: "+filepath.ToSlash(filepath.Join(root, "blobs"))+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := executeProject(t, "storage", "migrate", "--config", config); e != nil {
		t.Fatal(e)
	}
	handle := "pg:cli-" + filepath.Base(filepath.Dir(root))
	if _, e := executeProject(t, "init", handle, "--config", config); e != nil {
		t.Fatal(e)
	}
	data, e := executeProject(t, "inspect", handle, "--config", config)
	if e != nil {
		t.Fatal(e)
	}
	var v app.Inspection
	if e = json.Unmarshal([]byte(data), &v); e != nil || len(v.Pages) == 0 {
		t.Fatalf("inspection %v", e)
	}
}
