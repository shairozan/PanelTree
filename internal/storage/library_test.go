package storage

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedLibraryVersionPins(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	p, e := Connect(ctx, dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	id := filepath.Base(filepath.Dir(root))
	for _, suffix := range []string{"a", "b"} {
		if e = p.Import(ctx, id+suffix, source); e != nil {
			t.Fatal(e)
		}
	}
	v := LibraryVersion{ID: id, Version: "v1", Package: "characters/patrick.json"}
	files := map[string][]byte{v.Package: []byte(`{"id":"patrick"}`)}
	if e = p.PublishLibrary(ctx, v, files); e != nil {
		t.Fatal(e)
	}
	for _, suffix := range []string{"a", "b"} {
		if e = p.BindLibrary(ctx, id+suffix, id, "v1", func(w *workspace.Session, v LibraryVersion) error {
			data, e := os.ReadFile(filepath.Join(w.Root, v.Package))
			if string(data) != string(files[v.Package]) {
				t.Error("wrong shared bytes")
			}
			return e
		}); e != nil {
			t.Fatal(e)
		}
	}
	if e = p.PublishLibrary(ctx, v, files); e == nil {
		t.Fatal("published version overwritten")
	}
	if e = p.DeleteLibrary(ctx, id, "v1"); e == nil {
		t.Fatal("referenced version deleted")
	}
	v.Version = "v2"
	files[v.Package] = []byte(`{"id":"patrick","version":"v2"}`)
	if e = p.PublishLibrary(ctx, v, files); e != nil {
		t.Fatal(e)
	}
	if e = p.Open(ctx, id+"a", func(w *workspace.Session) error {
		data, e := os.ReadFile(filepath.Join(w.Root, v.Package))
		if string(data) != `{"id":"patrick"}` {
			t.Error("pin silently upgraded")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
}

func TestLibraryPortableBackupRestore(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	p, e := Connect(ctx, dsn, filepath.Join(root, "original-blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	id := filepath.Base(filepath.Dir(root))
	if e = p.Import(ctx, id, source); e != nil {
		t.Fatal(e)
	}
	v := LibraryVersion{ID: id, Version: "v1", Package: "characters/patrick.json"}
	if e = p.PublishLibrary(ctx, v, map[string][]byte{v.Package: []byte(`{"id":"patrick"}`)}); e != nil {
		t.Fatal(e)
	}
	if e = p.BindLibrary(ctx, id, id, "v1", func(*workspace.Session, LibraryVersion) error { return nil }); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(root, "backup")
	if e = p.Export(ctx, id, backup); e != nil {
		t.Fatal(e)
	}
	// Import into the same library must also verify the exported bytes.
	packagePath := filepath.Join(backup, v.Package)
	original, e := os.ReadFile(packagePath)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(packagePath, []byte(`{"id":"tampered"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e = p.Import(ctx, id+"tampered", filepath.Join(backup, "project.yaml")); e == nil {
		t.Error("tampered existing library accepted")
	}
	if e = os.WriteFile(packagePath, original, 0600); e != nil {
		t.Fatal(e)
	}
	schema := "restore_" + strings.ToLower(id)
	c, e := p.connect(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		_, _ = c.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		_ = c.Close(ctx)
	}()
	if _, e = c.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	cfg, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := cfg.Query()
	q.Set("search_path", schema)
	cfg.RawQuery = q.Encode()
	restored, e := Connect(ctx, cfg.String(), filepath.Join(root, "restored-blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = restored.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = restored.Import(ctx, id, filepath.Join(backup, "project.yaml")); e != nil {
		t.Fatal(e)
	}
	versions, e := restored.Library(ctx)
	if e != nil || len(versions) != 1 {
		t.Fatalf("library provenance lost: %v %v", versions, e)
	}
	if e = restored.DeleteLibrary(ctx, id, "v1"); e == nil {
		t.Fatal("restored binding missing")
	}
	if e = restored.Open(ctx, id, func(w *workspace.Session) error {
		b, e := os.ReadFile(filepath.Join(w.Root, v.Package))
		if string(b) != `{"id":"patrick"}` {
			t.Error("restored blob differs")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	// A future schema must reject every service entry, not just project opening.
	rc, e := restored.connect(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = rc.Exec(ctx, `UPDATE pt_schema SET version=999`); e != nil {
		t.Fatal(e)
	}
	_ = rc.Close(ctx)
	if e = restored.Migrate(ctx); e == nil {
		t.Error("future schema migrated")
	}
	if _, e = restored.Library(ctx); e == nil {
		t.Error("future schema library read accepted")
	}
	if _, e = restored.Projects(ctx); e == nil {
		t.Error("future schema project listing accepted")
	}
	store, e := restored.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.List(ctx); e == nil {
		t.Error("future schema jobs accepted")
	}

}
