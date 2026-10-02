package app

import (
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func postgresFixture(t *testing.T) (*Service, string) {
	t.Helper()
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("PANELTREE_TEST_POSTGRES not configured")
	}
	root := t.TempDir()
	p, e := storage.Connect(context.Background(), dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	if e = p.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, source, _ := editFixture(t)
	id := "app-" + filepath.Base(filepath.Dir(root))
	if e = p.Import(context.Background(), id, source); e != nil {
		t.Fatal(e)
	}
	return NewService(WithStorage(p)), "pg:" + id
}
func TestPostgresServiceEditBuild(t *testing.T) {
	s, handle := postgresFixture(t)
	ctx := context.Background()
	before, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	changed, e := s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: before.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}})
	if e != nil {
		t.Fatal(e)
	}
	after, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if after.Revision != changed.Revision || after.Layers["page-01/p1/hero"].Lock != model.AllLock {
		t.Fatal("database edit lost")
	}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: before.Revision, Operations: []Operation{{Target: target(), Action: "unlock"}}}); e == nil {
		t.Fatal("stale edit accepted")
	}
	if _, e = s.Build(ctx, BuildRequest{ProjectFile: handle, PageID: "page-01", Output: filepath.Join(t.TempDir(), "page.png")}); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresServiceJobs(t *testing.T) {
	s, handle := postgresFixture(t)
	ctx := context.Background()
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: handle, ExpectedRevision: view.Revision, Target: target(), IdempotencyKey: "pg-job", Renderer: "builtin", Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{handle, 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{ProjectFile: handle, JobID: j.ID, ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresLibraryApplication(t *testing.T) {
	s, handle := postgresFixture(t)
	_, source, _ := characterFixture(t)
	ctx := context.Background()
	id := strings.TrimPrefix(handle, "pg:")
	if _, e := s.Library(ctx, LibraryRequest{Action: "publish", ProjectFile: source, ID: id, Version: "v1", Package: "characters/alex.json"}); e != nil {
		t.Fatal(e)
	}
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: handle, ID: id, Version: "v1", Target: target(), ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
	after, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	node := indexLayers(after.Snapshot)["page-01/p1/hero"]
	found := node.layer.Source.Character != nil && strings.HasPrefix(node.layer.Source.Character.Package, "characters/library/")
	if !found {
		t.Fatal("library binding missing")
	}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: after.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}}); e != nil {
		t.Fatal(e)
	}
	locked, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "publish", ProjectFile: source, ID: id, Version: "v2", Package: "characters/alex.json"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: handle, ID: id, Version: "v2", Target: target(), ExpectedRevision: locked.Revision}); e == nil {
		t.Fatal("library update bypassed lock")
	}
}

func TestPostgresInspectionPaths(t *testing.T) {
	s, handle := postgresFixture(t)
	v, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range v.Documents {
		if filepath.IsAbs(d.File) {
			t.Errorf("temporary document path: %s", d.File)
		}
	}
	for _, d := range v.Pages {
		if filepath.IsAbs(d.File) {
			t.Errorf("temporary page path: %s", d.File)
		}
	}
	for _, d := range v.Scenes {
		if filepath.IsAbs(d.File) {
			t.Errorf("temporary scene path: %s", d.File)
		}
	}
}

func TestPostgresManualOverridePortable(t *testing.T) {
	ctx := context.Background()
	s, handle := postgresFixture(t)
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: view.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: "assets/hero.png"}}}); e != nil {
		t.Fatal(e)
	}
	current, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: current.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Build(ctx, BuildRequest{ProjectFile: handle, PageID: "page-01", Output: filepath.Join(t.TempDir(), "page.png")}); e != nil {
		t.Fatalf("override lost: %v", e)
	}
	_, source, _ := editFixture(t)
	local := NewService()
	before, e := local.Inspect(ctx, InspectRequest{ProjectFile: source})
	if e != nil {
		t.Fatal(e)
	}
	edited, e := local.Edit(ctx, EditRequest{ProjectFile: source, ExpectedRevision: before.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: "assets/hero.png"}}})
	if e != nil {
		t.Fatal(e)
	}
	edited, e = local.Edit(ctx, EditRequest{ProjectFile: source, ExpectedRevision: edited.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}})
	if e != nil {
		t.Fatal(e)
	}
	imported := strings.TrimPrefix(handle, "pg:") + "manual"
	if e = s.storage.Import(ctx, imported, source); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(t.TempDir(), "export")
	if e = s.storage.Export(ctx, imported, backup); e != nil {
		t.Fatal(e)
	}
	// Source ceases to exist; portable selection and its lock still work.
	if e = os.Rename(filepath.Dir(source), filepath.Dir(source)+"-moved"); e != nil {
		t.Fatal(e)
	}
	restored, e := local.Inspect(ctx, InspectRequest{ProjectFile: filepath.Join(backup, "project.yaml")})
	if e != nil {
		t.Fatal(e)
	}
	if restored.Revision != edited.Revision {
		t.Fatal("portable override changed editorial revision")
	}
	if _, e = local.Build(ctx, BuildRequest{ProjectFile: filepath.Join(backup, "project.yaml"), PageID: "page-01", Output: filepath.Join(t.TempDir(), "page.png")}); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresPublishedReferencesRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, handle := postgresFixture(t)
	_, source, _ := characterFixture(t)
	r := ReferenceRequest{ProjectFile: source, Set: "hero", Action: "create", License: "fixture-license", Attribution: "original artist"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for _, slot := range []string{"body/front", "head/front", "body/left", "body/right", "body/rear", "head/left", "head/right", "head/rear"} {
		r.Action = "import"
		r.Slot = slot
		r.Path = "assets/hero.png"
		r.Revision = set.Revision
		set, e = s.Reference(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
		for id, c := range set.Candidates {
			if c.Slot == slot {
				r.Candidate = id
			}
		}
		r.Action = "accept"
		r.Revision = set.Revision
		set, e = s.Reference(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
	}
	r.Action = "publish"
	r.Version = "1"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	id := strings.TrimPrefix(handle, "pg:")
	if _, e = s.Library(ctx, LibraryRequest{Action: "publish", ProjectFile: source, ID: id, Version: "refs-v1", Package: "characters/alex.json", Set: "hero", ReferenceVersion: "1"}); e != nil {
		t.Fatal(e)
	}

	details, e := s.LibraryCharacter(ctx, id, "refs-v1")
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(details)
	if !strings.Contains(string(encoded), `"published_cards"`) {
		t.Fatal("published reference cards missing from library details")
	}
	before, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: handle, ID: id, Version: "refs-v1", Target: target(), ExpectedRevision: before.Revision}); e != nil {
		t.Fatal(e)
	}
	bound, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	use := indexLayers(bound.Snapshot)["page-01/p1/hero"].layer.Source.Character
	boundSet, e := s.Reference(ctx, ReferenceRequest{ProjectFile: handle, Set: use.ReferenceSet.Set, Action: "inspect"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(set.Published["1"], boundSet.Published["1"]) {
		t.Fatal("publication metadata or assets changed on reuse")
	}
	// A second project binds the exact same immutable publication.
	second := handle + "second"
	if _, e = s.Init(ctx, InitRequest{Directory: second}); e != nil {
		t.Fatal(e)
	}
	other, e := s.Inspect(ctx, InspectRequest{ProjectFile: second})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: second, ID: id, Version: "refs-v1", Target: target(), ExpectedRevision: other.Revision}); e != nil {
		t.Fatal(e)
	}
	reused, e := s.Reference(ctx, ReferenceRequest{ProjectFile: second, Set: use.ReferenceSet.Set, Action: "inspect"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(reused, boundSet) {
		t.Fatal("second project reference version differs")
	}
	// Pin and lock one layer, export, and exercise the selected references from the file backend.
	reviewed, e := s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: bound.Revision, Operations: []Operation{{Target: target(), Action: "review"}}})
	if e != nil {
		t.Fatal(e)
	}
	approved, e := s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: reviewed.Revision, Operations: []Operation{{Target: target(), Action: "approve", Artifact: "assets/hero.png"}}})
	if e != nil {
		t.Fatal(e)
	}
	locked, e := s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: approved.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}})
	if e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(t.TempDir(), "portable")
	if e = s.storage.Export(ctx, id, dest); e != nil {
		t.Fatal(e)
	}
	local := NewService()
	entry := filepath.Join(dest, "project.yaml")
	restored, e := local.Inspect(ctx, InspectRequest{ProjectFile: entry})
	if e != nil {
		t.Fatal(e)
	}
	if restored.Revision != locked.Revision || !reflect.DeepEqual(restored.Layers, locked.Layers) {
		t.Fatal("editorial state changed on export")
	}
	refs, e := local.Reference(ctx, ReferenceRequest{ProjectFile: entry, Set: use.ReferenceSet.Set, Action: "inspect"})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(refs, boundSet) {
		t.Fatal("reference approvals/provenance changed")
	}
	if _, e = local.Build(ctx, BuildRequest{ProjectFile: entry, PageID: "page-01", Output: filepath.Join(t.TempDir(), "page.png")}); e != nil {
		t.Fatal(e)
	}
	if e = s.storage.Import(ctx, id+"restored", entry); e != nil {
		t.Fatal(e)
	}
	restored, e = s.Inspect(ctx, InspectRequest{ProjectFile: "pg:" + id + "restored"})
	if e != nil {
		t.Fatal(e)
	}
	if restored.Revision != locked.Revision {
		t.Fatal("reimport revision changed")
	}
}

func TestPortableManualOverrideReselection(t *testing.T) {
	ctx := context.Background()
	s, source, view := editFixture(t)
	path := filepath.Join(filepath.Dir(source), "assets/hero.png")
	if _, e := s.Edit(ctx, EditRequest{ProjectFile: source, ExpectedRevision: view.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: path}}}); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(t.TempDir(), "copy")
	if e := (storage.Filesystem{}).Import(ctx, dest, source); e != nil {
		t.Fatal(e)
	}
	replacement, e := os.ReadFile(filepath.Join(filepath.Dir(source), "assets/setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, replacement, 0600); e != nil {
		t.Fatal(e)
	}
	entry := filepath.Join(dest, "project.yaml")
	view, e = s.Inspect(ctx, InspectRequest{ProjectFile: entry})
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.Edit(ctx, EditRequest{ProjectFile: entry, ExpectedRevision: view.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: path}}})
	if e != nil {
		t.Fatal(e)
	}
	selected, e := asset.ManualPath(dest, result.Layers["page-01/p1/hero"].Manual)
	if e != nil {
		t.Fatal(e)
	}
	data, e := asset.ReadPNG(selected)
	if e != nil {
		t.Fatal(e)
	}
	if asset.Digest(data) != asset.Digest(replacement) {
		t.Fatal("explicit override ignored due to old portable mapping")
	}
}
func TestPostgresManualSelectionsAreIndependent(t *testing.T) {
	ctx := context.Background()
	s, handle := postgresFixture(t)
	_, source, _ := editFixture(t)
	external := filepath.Join(filepath.Dir(source), "assets/hero.png")
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	other := LayerTarget{Page: "page-01", Panel: "p1"}
	for _, l := range view.Pages[0].Page.Panels[0].Layers {
		if l.Source != nil {
			other.Layer = l.ID
			break
		}
	}
	if other.Layer == "" {
		t.Fatal("no second leaf fixture")
	}
	result, e := s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: view.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: external}, {Target: other, Action: "override", Artifact: external}}})
	if e != nil {
		t.Fatal(e)
	}
	result, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: result.Revision, Operations: []Operation{{Target: other, Action: "lock", Scope: model.AssetLock}}})
	if e != nil {
		t.Fatal(e)
	}
	replacement, e := os.ReadFile(filepath.Join(filepath.Dir(source), "assets/setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(external, replacement, 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: result.Revision, Operations: []Operation{{Target: target(), Action: "override", Artifact: external}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Inspect(ctx, InspectRequest{ProjectFile: handle}); e != nil {
		t.Fatalf("unlocked override changed locked sibling: %v", e)
	}
}

func TestFileProjectLibraryApplication(t *testing.T) {
	s, dbHandle := postgresFixture(t)
	_, handle, _ := editFixture(t)
	_, source, _ := characterFixture(t)
	ctx := context.Background()
	id := strings.TrimPrefix(dbHandle, "pg:") + "-files"
	if _, e := s.Library(ctx, LibraryRequest{Action: "publish", ProjectFile: source, ID: id, Version: "v1", Package: "characters/alex.json"}); e != nil {
		t.Fatal(e)
	}
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: handle, ID: id, Version: "v1", Target: target(), ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
	after, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	node := indexLayers(after.Snapshot)["page-01/p1/hero"]
	found := node.layer.Source.Character != nil && strings.HasPrefix(node.layer.Source.Character.Package, "characters/library/")
	if !found {
		t.Fatal("library binding missing")
	}
	if e = s.storage.Import(ctx, id+"-restored", handle); e != nil {
		t.Fatal(e)
	}
	if e = s.storage.DeleteLibrary(ctx, id, "v1"); e == nil {
		t.Fatal("portable file binding lost on import")
	}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: handle, ExpectedRevision: after.Revision, Operations: []Operation{{Target: target(), Action: "lock", Scope: model.AllLock}}}); e != nil {
		t.Fatal(e)
	}
	locked, e := s.Inspect(ctx, InspectRequest{ProjectFile: handle})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "publish", ProjectFile: source, ID: id, Version: "v2", Package: "characters/alex.json"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Library(ctx, LibraryRequest{Action: "use", ProjectFile: handle, ID: id, Version: "v2", Target: target(), ExpectedRevision: locked.Revision}); e == nil {
		t.Fatal("library update bypassed lock")
	}
}
