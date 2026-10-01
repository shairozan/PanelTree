package storage

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectContract(t *testing.T) {
	for _, backend := range []string{"files", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			source, e := project.Init(filepath.Join(root, "source"))
			if e != nil {
				t.Fatal(e)
			}
			var store ProjectStore = Filesystem{}
			handle := filepath.Join(root, "copy")
			if backend == "postgres" {
				dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
				if dsn == "" {
					t.Skip("PANELTREE_TEST_POSTGRES not configured")
				}
				p, e := Connect(ctx, dsn, filepath.Join(root, "blobs"))
				if e != nil {
					t.Fatal(e)
				}
				defer p.Close()
				if e = p.Migrate(ctx); e != nil {
					t.Fatal(e)
				}
				store = p
				handle = "contract-" + filepath.Base(filepath.Dir(root))
			}
			if e = store.Import(ctx, handle, source); e != nil {
				t.Fatal(e)
			}
			var revision string
			if e = store.Open(ctx, handle, func(w *workspace.Session) error {
				revision = string(w.Snapshot.Revision)
				if len(w.Snapshot.Pages) == 0 {
					t.Error("missing pages")
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			if e = store.Import(ctx, handle, source); e == nil {
				t.Fatal("collision overwritten")
			}
			dest := filepath.Join(root, "export")
			if e = store.Export(ctx, handle, dest); e != nil {
				t.Fatal(e)
			}
			if e = workspace.Open(ctx, filepath.Join(dest, "project.yaml"), func(w *workspace.Session) error {
				if string(w.Snapshot.Revision) != revision {
					t.Error("round trip changed revision")
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestPortableAssetPaths(t *testing.T) {
	ctx := context.Background()
	for _, outside := range []bool{false, true} {
		t.Run(fmt.Sprint(outside), func(t *testing.T) {
			root := t.TempDir()
			source, e := project.Init(filepath.Join(root, "source"))
			if e != nil {
				t.Fatal(e)
			}
			page := filepath.Join(filepath.Dir(source), "pages/01.yaml")
			data, e := os.ReadFile(page)
			if e != nil {
				t.Fatal(e)
			}
			asset := filepath.Join(filepath.Dir(source), "assets/hero.png")
			replacement := "../assets/hero.art"
			if outside {
				replacement = "../../outside.png"
			}
			destAsset := filepath.Join(filepath.Dir(source), "assets/hero.art")
			if outside {
				destAsset = filepath.Join(root, "outside.png")
			}
			b, e := os.ReadFile(asset)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(destAsset, b, 0600); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(page, []byte(strings.ReplaceAll(string(data), "../assets/hero.png", replacement)), 0600); e != nil {
				t.Fatal(e)
			}
			dest := filepath.Join(root, "copy")
			e = (Filesystem{}).Import(ctx, dest, source)
			if outside {
				if e == nil {
					t.Fatal("external dependency accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = os.Stat(filepath.Join(dest, "assets/hero.art")); e != nil {
				t.Fatal("referenced nonstandard asset extension lost")
			}
		})
	}
}

func TestAlternateOwnerFilename(t *testing.T) {
	root := t.TempDir()
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	renamed := filepath.Join(filepath.Dir(source), "book.yaml")
	if e = os.Rename(source, renamed); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(root, "copy")
	if e = (Filesystem{}).Import(context.Background(), dest, renamed); e != nil {
		t.Fatal(e)
	}
	if _, e = project.Load(filepath.Join(dest, "project.yaml")); e != nil {
		t.Fatal(e)
	}
}

func TestPortableCharacterDependencies(t *testing.T) {
	root := t.TempDir()
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	base := filepath.Dir(source)
	if e = os.MkdirAll(filepath.Join(base, "characters"), 0700); e != nil {
		t.Fatal(e)
	}
	for path, data := range map[string]string{"characters/private.env": "DO_NOT_EXPORT=credential", "characters/hero.character": `{"schema":"paneltree/character/v1","id":"hero","version":"1","references":[{"path":"assets/portrait.custom"}]}`, "assets/portrait.custom": "reference-bytes"} {
		if e = os.WriteFile(filepath.Join(base, path), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	page := filepath.Join(base, "pages/01.yaml")
	var b []byte
	snap, e := project.Load(source)
	if e != nil {
		t.Fatal(e)
	}
	d := snap.Documents[2].Document
	var visit func([]model.Layer)
	visit = func(ls []model.Layer) {
		for i := range ls {
			if ls[i].ID == "hero" {
				ls[i].Source.Character = &model.CharacterUse{Package: "characters/hero.character"}
			}
			visit(ls[i].Children)
		}
	}
	for i := range d.Page.Panels {
		visit(d.Page.Panels[i].Layers)
	}
	b, e = yaml.Marshal(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(page, b, 0600); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(root, "export")
	if e = (Filesystem{}).Import(context.Background(), dest, source); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dest, "characters/private.env")); !os.IsNotExist(e) {
		t.Error("runtime credential file copied")
	}
	if b, e = os.ReadFile(filepath.Join(dest, "assets/portrait.custom")); e != nil || string(b) != "reference-bytes" {
		t.Error("custom-extension character dependency lost")
	}
}

func TestPostgresRollbackAndMissingBlob(t *testing.T) {
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
	id := "rollback-" + filepath.Base(filepath.Dir(root))
	if e = p.Import(ctx, id, source); e != nil {
		t.Fatal(e)
	}
	var revision model.Revision
	if e = p.Open(ctx, id, func(w *workspace.Session) error { revision = w.Snapshot.Revision; return nil }); e != nil {
		t.Fatal(e)
	}
	e = p.Open(ctx, id, func(w *workspace.Session) error {
		if err := os.WriteFile(filepath.Join(w.Root, "project.yaml"), []byte("invalid pending edit"), 0600); err != nil {
			return err
		}
		return fmt.Errorf("interrupted before publication")
	})
	if e == nil {
		t.Fatal("interruption accepted")
	}
	if e = p.Open(ctx, id, func(w *workspace.Session) error {
		if revision != w.Snapshot.Revision {
			t.Error("partial publication")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(filepath.Dir(source), "assets/hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	hash := digest(data)
	if e = os.Remove(filepath.Join(p.root, "blobs", hash)); e != nil {
		t.Fatal(e)
	}
	if e = p.Open(ctx, id, func(*workspace.Session) error { return nil }); e == nil || !strings.Contains(e.Error(), "missing blob") {
		t.Fatalf("missing original not diagnosed: %v", e)
	}
	if _, e = p.put(data); e != nil {
		t.Fatal(e)
	}
	if e = p.Open(ctx, id, func(w *workspace.Session) error {
		if revision != w.Snapshot.Revision {
			t.Error("failed read changed project")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
