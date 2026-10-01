package app

import (
	"context"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"testing"
)

func TestReferenceApprovalAndPublication(t *testing.T) {
	s, p, _ := editFixture(t)
	r := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "create", License: "fixture", Attribution: "PanelTree test fixture"}
	set, e := s.Reference(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	call := func(action, slot, path string) ReferenceSet {
		t.Helper()
		r.Action = action
		r.Slot = slot
		r.Path = path
		r.Revision = set.Revision
		next, e := s.Reference(context.Background(), r)
		if e != nil {
			t.Fatal(e)
		}
		set = next
		return next
	}
	call("import", "body/front", "assets/hero.png")
	if len(set.Accepted) != 0 {
		t.Fatal("import automatically approved")
	}
	for id := range set.Candidates {
		r.Candidate = id
	}
	call("accept", "body/front", "")
	call("import", "body/front", "assets/hero.png")
	for _, slot := range []string{"head/front", "body/left", "body/right", "body/rear", "head/left", "head/right", "head/rear"} {
		call("import", slot, "assets/hero.png")
		for id, c := range set.Candidates {
			if c.Slot == slot {
				r.Candidate = id
			}
		}
		call("accept", slot, "")
	}
	r.Version = "1"
	call("publish", "", "")
	if len(set.Published["1"].Cards) != 4 || len(set.Published["1"].Accepted) != 8 {
		t.Fatal("incomplete publication")
	}
	r.Candidate = ""
	call("import", "body/front", "assets/setting.png")
	for id, c := range set.Candidates {
		if c.Slot == "body/front" && id != set.Accepted["body/front"] {
			r.Candidate = id
		}
	}
	call("accept", "body/front", "")
	if len(set.Accepted) != 1 {
		t.Fatalf("descendant approvals survived replacement: %v", set.Accepted)
	}
	if len(set.Published["1"].Accepted) != 8 {
		t.Fatal("published version mutated")
	}
	r.Action = "publish"
	r.Version = "2"
	r.Revision = set.Revision
	if _, e = s.Reference(context.Background(), r); e == nil {
		t.Fatal("partial reference set published")
	}
	r.Action = "inspect"
	resumed, e := NewService().Reference(context.Background(), r)
	if e != nil || resumed.Revision != set.Revision {
		t.Fatalf("resume: %v", e)
	}
}

func TestPanelRejectsMissingPublishedReferences(t *testing.T) {
	s, p, i := characterFixture(t)
	d := i.Documents[2].Document
	// Attach a version that does not exist; generation must never fall back to text.
	d.Page.Panels[0].Layers[1].Children[0].Source.Character.ReferenceSet = &model.ReferenceSelection{Set: "missing", Version: "1", Directions: []string{"front"}, Packing: "cards-row/v1"}
	edit, e := s.Edit(context.Background(), EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: d}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RequestAsset(context.Background(), characterRequest(p, edit.Revision, "missing-refs", target())); e == nil {
		t.Fatal("missing published references silently replaced with text")
	}
}

func TestRejectedImportCanBeRetried(t *testing.T) {
	s, p, _ := editFixture(t)
	ctx := context.Background()
	r := ReferenceRequest{ProjectFile: p, Set: "retry", Action: "create", License: "fixture", Attribution: "fixture"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Slot = "body/front"
	r.Path = "assets/hero.png"
	r.Action = "import"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for id := range set.Candidates {
		r.Candidate = id
	}
	r.Action = "reject"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Action = "import"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for id, c := range set.Candidates {
		if !c.Rejected {
			r.Candidate = id
			found = true
		}
	}
	if !found {
		t.Fatal("reimport cannot recover rejected artwork")
	}
	r.Action = "accept"
	r.Revision = set.Revision
	if _, e = s.Reference(ctx, r); e != nil {
		t.Fatal(e)
	}
}

func TestReferenceGenerationExplicitAcceptance(t *testing.T) {
	_, p, _ := editFixture(t)
	c, posts := generationFixture(t)
	s := NewService(WithComfyUI(c))
	ctx := context.Background()
	r := ReferenceRequest{ProjectFile: p, Set: "generated", Action: "create"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Action = "request"
	r.Revision = set.Revision
	r.Slot = "body/front"
	r.Key = "front-1"
	r.Width = 64
	r.Height = 64
	r.Generation = &render.Generation{Prompt: "blue coat, scar on left cheek", Seed: 4}
	r.License = "model terms"
	r.Attribution = "fixture"
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.Reference(ctx, r)
	if e != nil || duplicate.Jobs[r.Key] != set.Jobs[r.Key] {
		t.Fatalf("retry: %v", e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	r.Action = "collect"
	r.JobID = set.Jobs[r.Key]
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if len(set.Accepted) != 0 || len(set.Candidates) != 1 || posts.Load() != 1 {
		t.Fatalf("generation selected or duplicated: %+v", set)
	}
	for id := range set.Candidates {
		r.Candidate = id
	}
	r.Action = "accept"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil || set.Accepted["body/front"] != r.Candidate {
		t.Fatalf("accept: %v", e)
	}
}
