package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupLocksProtectDescendants(t *testing.T) {
	for _, action := range []string{"placement", "selection"} {
		t.Run(action, func(t *testing.T) {
			s, p, i := editFixture(t)
			group := target()
			group.Layer = "cast"
			r := applyOp(t, s, p, i.Revision, Operation{Target: group, Action: "lock"})
			req := EditRequest{ProjectFile: p, ExpectedRevision: r.Revision}
			if action == "placement" {
				d := i.Documents[2].Document
				d.Page.Panels[0].Layers[1].Children[0].Frame.X += 0.1
				req.Edits = []DocumentEdit{{File: "pages/01.yaml", Document: d}}
			} else {
				req.Operations = []Operation{{Target: target(), Action: "override", Artifact: "assets/hero.png"}}
			}
			if _, e := s.Edit(context.Background(), req); e == nil {
				t.Fatal("group lock bypassed by descendant " + action)
			}
		})
	}
}
func TestLockedSourceReplacementRejectsWrites(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock", Scope: model.AssetLock})
	replacement, e := os.ReadFile(filepath.Join(filepath.Dir(p), "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "assets", "hero.png"), replacement, 0600); e != nil {
		t.Fatal(e)
	}
	d := i.Documents[0].Document
	d.Book.Title = "unrelated"
	if _, e = s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "project.yaml", Document: d}}}); e == nil {
		t.Fatal("source replacement bypassed asset lock on write")
	}
}
func TestInspectKeepsAuthoringSource(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "review"})
	applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "approve", Artifact: "assets/hero.png"})
	view, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(i.Documents[2].Document)
	b, _ := json.Marshal(view.Documents[2].Document)
	if string(a) != string(b) {
		t.Fatal("inspection replaced authoring source with selection")
	}
}

func editFixture(t *testing.T) (*Service, string, *Inspection) {
	t.Helper()
	s := NewService()
	r, e := s.Init(context.Background(), InitRequest{filepath.Join(t.TempDir(), "book")})
	if e != nil {
		t.Fatal(e)
	}
	i, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: r.ProjectFile})
	if e != nil {
		t.Fatal(e)
	}
	return s, r.ProjectFile, i
}
func target() LayerTarget { return LayerTarget{"page-01", "p1", "hero"} }
func applyOp(t *testing.T, s *Service, p string, rev model.Revision, op Operation) EditResult {
	t.Helper()
	r, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: rev, Operations: []Operation{op}})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestEditorialApproval(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	tar := target()
	key := "page-01/p1/hero"
	if _, e := s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Operations: []Operation{{Target: tar, Action: "approve", Artifact: "assets/hero.png"}}}); e == nil {
		t.Fatal("draft approved without review")
	}
	r := applyOp(t, s, p, i.Revision, Operation{Target: tar, Action: "review"})
	r = applyOp(t, s, p, r.Revision, Operation{Target: tar, Action: "approve", Artifact: "assets/hero.png"})
	if r.Layers[key].Pin == "" || r.Layers[key].State != model.Approved {
		t.Fatalf("missing pin: %+v", r)
	}
	d := i.Documents[2].Document
	d.Page.Panels[0].Layers[1].Children[0].Source.Draft.Revision++
	r, e := s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
	if e != nil {
		t.Fatal(e)
	}
	if !r.Layers[key].Stale || r.Layers[key].Pin == "" {
		t.Fatal("approved selection lost or stale provenance missing")
	}
	pin := filepath.Join(filepath.Dir(p), ".paneltree", "assets", r.Layers[key].Pin+".png")
	if e = os.WriteFile(pin, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "page-01", Output: filepath.Join(filepath.Dir(p), "result.png"), Width: 120, Height: 180})
	if e == nil || !strings.Contains(e.Error(), "pin") {
		t.Fatalf("corrupt pin did not fail closed: %v", e)
	}
}
func TestScopedLocks(t *testing.T) {
	for _, scope := range []model.LockScope{model.AssetLock, model.PlacementLock, ""} {
		t.Run(string(scope), func(t *testing.T) {
			s, p, i := editFixture(t)
			r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock", Scope: scope})
			d := i.Documents[2].Document
			d.Page.Panels[0].Layers[1].Children[0].Source.Draft.Revision++
			_, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
			if (e == nil) != (scope == model.PlacementLock) {
				t.Fatalf("asset protection for %s: %v", scope, e)
			}
			// Fresh fixture tests indirect placement independently.
			s, p, i = editFixture(t)
			r = applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock", Scope: scope})
			d = i.Documents[2].Document
			d.Page.Layout.Margin++
			_, e = s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
			if (e == nil) != (scope == model.AssetLock) {
				t.Fatalf("ancestor placement protection for %s: %v", scope, e)
			}
		})
	}
}
func TestManualOverridePreserved(t *testing.T) {
	s, p, i := editFixture(t)
	path := filepath.Join(filepath.Dir(p), "assets", "hero.png")
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "override", Artifact: "assets/hero.png"})
	if r.Layers["page-01/p1/hero"].Manual == "" {
		t.Fatal("override missing")
	}
	_, e = s.Build(context.Background(), BuildRequest{ProjectFile: p, PageID: "page-01", Output: filepath.Join(filepath.Dir(p), "manual.png"), Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("manual override overwritten")
	}
}

func TestPlacementLockAllowsSelection(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock", Scope: model.PlacementLock})
	r = applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "override", Artifact: "assets/hero.png"})
	if r.Layers["page-01/p1/hero"].Lock != model.PlacementLock {
		t.Fatal("selection cleared placement lock")
	}
}

func TestGroupLockChecksManualBytes(t *testing.T) {
	s, p, i := editFixture(t)
	root := filepath.Dir(p)
	manual := filepath.Join(root, "manual.png")
	data, e := os.ReadFile(filepath.Join(root, "assets", "hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(manual, data, 0600); e != nil {
		t.Fatal(e)
	}
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "override", Artifact: manual})
	group := target()
	group.Layer = "cast"
	r = applyOp(t, s, p, r.Revision, Operation{Target: group, Action: "lock", Scope: model.AssetLock})
	data, e = os.ReadFile(filepath.Join(root, "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(manual, data, 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.Build(context.Background(), BuildRequest{ProjectFile: p, PageID: "page-01", Output: filepath.Join(root, "out.png"), Width: 120, Height: 180})
	if e == nil {
		t.Fatal("group asset lock ignored replaced manual artwork")
	}
}
func TestAllLockChecksMaskBytes(t *testing.T) {
	s, p, i := editFixture(t)
	root := filepath.Dir(p)
	mask := filepath.Join(root, "mask.png")
	data, e := os.ReadFile(filepath.Join(root, "assets", "hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(mask, data, 0600); e != nil {
		t.Fatal(e)
	}
	d := i.Documents[2].Document
	d.Page.Panels[0].Layers[1].Children[0].Mask = "../mask.png"
	r, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
	if e != nil {
		t.Fatal(e)
	}
	r = applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "lock"})
	data, e = os.ReadFile(filepath.Join(root, "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(mask, data, 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.Build(context.Background(), BuildRequest{ProjectFile: p, PageID: "page-01", Output: filepath.Join(root, "out.png"), Width: 120, Height: 180})
	if e == nil {
		t.Fatal("all lock ignored mask replacement")
	}
}

func TestLockedDeletionFailsClosed(t *testing.T) {
	s, p, i := editFixture(t)
	applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock"})
	d := i.Documents[2].Document
	layers := d.Page.Panels[0].Layers
	d.Page.Panels[0].Layers = append(layers[:1], layers[2:]...)
	data, e := yaml.Marshal(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "pages", "01.yaml"), data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Validate(context.Background(), InspectRequest{ProjectFile: p}); e == nil {
		t.Fatal("external deletion bypassed lock")
	}
}

type failRaster struct{ calls int }

func (f *failRaster) Raster(context.Context, model.Source, string) (image.Image, error) {
	f.calls++
	return nil, fmt.Errorf("unexpected rasterization")
}
func TestMissingPinNeverRenders(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "review"})
	r = applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "approve", Artifact: "assets/hero.png"})
	if e := os.Remove(filepath.Join(filepath.Dir(p), ".paneltree", "assets", r.Layers["page-01/p1/hero"].Pin+".png")); e != nil {
		t.Fatal(e)
	}
	raster := &failRaster{}
	_, e := NewService(WithRasterizer(raster)).Build(context.Background(), BuildRequest{ProjectFile: p, PageID: "page-01", Output: filepath.Join(filepath.Dir(p), "missing.png"), Width: 120, Height: 180})
	if e == nil || !strings.Contains(e.Error(), "pin") || raster.calls != 0 {
		t.Fatalf("missing pin: err=%v raster calls=%d", e, raster.calls)
	}
}
func TestPinnedBuildRetainsApprovedArtwork(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	root := filepath.Dir(p)
	build := func(name string) []byte {
		t.Helper()
		out := filepath.Join(root, name+".png")
		if _, e := s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "page-01", Output: out, Width: 120, Height: 180}); e != nil {
			t.Fatal(e)
		}
		data, e := os.ReadFile(out)
		if e != nil {
			t.Fatal(e)
		}
		return data
	}
	original := build("original")
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "review"})
	r = applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "approve", Artifact: "assets/setting.png"})
	approved := build("approved")
	if bytes.Equal(original, approved) {
		t.Fatal("approved selection was not used")
	}
	d := i.Documents[2].Document
	d.Page.Panels[0].Layers[1].Children[0].Source.Draft.Seed++
	if _, e := s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}}); e != nil {
		t.Fatal(e)
	}
	if e := os.RemoveAll(filepath.Join(root, ".paneltree", "cache")); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(approved, build("retained")) {
		t.Fatal("request change or cache cleanup replaced approved artwork")
	}
}

func TestLockAndSelectionChangesetOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			s, p, i := editFixture(t)
			group := target()
			group.Layer = "cast"
			ops := []Operation{{Target: group, Action: "lock"}, {Target: target(), Action: "override", Artifact: "assets/setting.png"}}
			if reverse {
				ops[0], ops[1] = ops[1], ops[0]
			}
			if _, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Operations: ops}); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: p}); e != nil {
				t.Fatalf("committed inconsistent changeset: %v", e)
			}
		})
	}
}
func TestUnlockChildPreservesAncestorSelection(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "override", Artifact: "assets/hero.png"})
	r = applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "lock", Scope: model.AssetLock})
	group := target()
	group.Layer = "cast"
	r = applyOp(t, s, p, r.Revision, Operation{Target: group, Action: "lock", Scope: model.AssetLock})
	applyOp(t, s, p, r.Revision, Operation{Target: target(), Action: "unlock"})
}
func TestStandaloneLockedPageRenameFailsClosed(t *testing.T) {
	s, p, i := editFixture(t)
	standalone := filepath.Join(filepath.Dir(p), "book.yml")
	if e := os.Rename(p, standalone); e != nil {
		t.Fatal(e)
	}
	applyOp(t, s, standalone, i.Revision, Operation{Target: target(), Action: "lock"})
	d := i.Documents[2].Document
	d.Page.ID = "renamed"
	data, e := yaml.Marshal(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "pages", "01.yaml"), data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Validate(context.Background(), InspectRequest{ProjectFile: standalone}); e == nil {
		t.Fatal("standalone root ignored missing locked page")
	}
}

func TestAssetLockProtectsSelectionReparenting(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "override", Artifact: "assets/setting.png"})
	group := target()
	group.Layer = "cast"
	r = applyOp(t, s, p, r.Revision, Operation{Target: group, Action: "lock", Scope: model.AssetLock})
	d := i.Documents[2].Document
	hero := d.Page.Panels[0].Layers[1].Children[0]
	replacement := hero
	replacement.ID = "hero-copy"
	d.Page.Panels[0].Layers[1].Children[0] = replacement
	d.Page.Panels[0].Layers = append(d.Page.Panels[0].Layers, hero)
	if _, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: r.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}}); e == nil {
		t.Fatal("selected artwork moved out of asset-locked group")
	}
	view, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	if view.Revision != r.Revision {
		t.Fatal("rejected reparenting changed revision")
	}
}
