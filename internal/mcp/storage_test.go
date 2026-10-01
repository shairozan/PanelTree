package mcp

import (
	"context"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestPostgresMCPProject(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	root := t.TempDir()
	p, e := storage.Connect(context.Background(), dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	service := app.NewService(app.WithStorage(p))
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
	c, e := protocol.NewClient(&protocol.Implementation{Name: "pg-test", Version: "1"}, nil).Connect(context.Background(), b, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Close() }()
	var init app.InitResult
	handle := "pg:mcp-" + filepath.Base(filepath.Dir(root))
	call(t, c, "project_init", map[string]any{"directory": handle}, &init)
	var view app.Inspection
	call(t, c, "project_open", map[string]any{"project_file": handle}, &view)
	if len(view.Pages) == 0 {
		t.Fatal("missing PostgreSQL pages")
	}
	var list app.StorageResult
	call(t, c, "storage_manage", map[string]any{"action": "list"}, &list)
	found := false
	for _, id := range list.Projects {
		if id == handle {
			found = true
		}
	}
	if !found {
		t.Fatal("project not discoverable")
	}
}
