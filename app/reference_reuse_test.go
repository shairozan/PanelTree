package app

import (
	"context"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/model"
	"testing"
)

func completeReferenceFixture(t *testing.T, s *Service, p string) ReferenceSet {
	t.Helper()
	ctx := context.Background()
	r := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "create", License: "CC0-1.0", Attribution: "PanelTree authored fixture"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for _, slot := range referenceSlots {
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
	return set
}

func TestPublishedReferencesFeedTwoPanels(t *testing.T) {
	_, p, i := characterFixture(t)
	c, posts := generationFixture(t)
	c.Profile.ImageBindings = []adapters.ImageBinding{{Role: "reference", Node: "image"}}
	c.Profile.Workflow["image"] = adapters.ComfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": ""}}
	c.Profile.Workflow["out"].Inputs["images"] = []any{"image", 0}
	s := NewService(WithComfyUI(c))
	if cap := s.Renderers()[1].ImageConditioning; cap == nil || cap.MaxDimension != 8192 {
		t.Fatal("missing image dimension capability")
	}
	set := completeReferenceFixture(t, s, p)
	d := i.Documents[2].Document
	selection := &model.ReferenceSelection{Set: "hero", Version: "1", Directions: []string{"front", "left", "right", "rear"}, Packing: "cards-row/v1"}
	d.Page.Panels[0].Layers[1].Children[0].Source.Character.ReferenceSet = selection
	// Both targets in the character fixture share the same package.
	for pi := range d.Page.Panels {
		for li := range d.Page.Panels[pi].Layers {
			l := &d.Page.Panels[pi].Layers[li]
			if l.Source != nil && l.Source.Character != nil {
				l.Source.Character.ReferenceSet = selection
			}
		}
	}
	edit, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
	if e != nil {
		t.Fatal(e)
	}
	for n, target := range []LayerTarget{target(), {"page-01", "p2", "closeup"}} {
		key := []string{"one", "two"}[n]
		j, e := s.RequestAsset(context.Background(), characterRequest(p, edit.Revision, key, target))
		if e != nil {
			t.Fatal(e)
		}
		if len(j.GenerationProvenance.ImageInputs) != 1 {
			t.Fatal("reference pixels missing from panel provenance")
		}
	}
	if _, e = s.RunJobs(context.Background(), RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	list, e := s.Jobs(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	for _, j := range list {
		if j.State != "succeeded" {
			t.Fatalf("panel failed: %+v", j)
		}
	}
	after, e := s.Reference(context.Background(), ReferenceRequest{ProjectFile: p, Set: "hero", Action: "inspect"})
	if e != nil {
		t.Fatal(e)
	}
	if after.Revision != set.Revision || posts.Load() != 2 {
		t.Fatal("panel generation mutated or regenerated canonical references")
	}
}
