package app

import (
	"context"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/render"
	"testing"
)

func TestReferenceStaleResultAndCancellation(t *testing.T) {
	_, p, _ := editFixture(t)
	c, _ := generationFixture(t)
	c.Profile.ImageBindings = []adapters.ImageBinding{{Role: "reference", Node: "image"}}
	c.Profile.Workflow["image"] = adapters.ComfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": ""}}
	c.Profile.Workflow["out"].Inputs["images"] = []any{"image", 0}
	s := NewService(WithComfyUI(c))
	ctx := context.Background()
	set := completeReferenceFixture(t, s, p)
	req := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "request", Revision: set.Revision, Slot: "body/left", Key: "left", Width: 64, Height: 64, Generation: &render.Generation{Prompt: "left profile", Seed: 4}, License: "fixture", Attribution: "fixture"}
	set, e := s.Reference(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	jobID := set.Jobs["left"]
	// Independently replace the front while the old-parent request is queued.
	r := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "import", Revision: set.Revision, Slot: "body/front", Path: "assets/setting.png", License: "fixture", Attribution: "fixture"}
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for id, c := range set.Candidates {
		if c.Slot == "body/front" && id != set.Accepted["body/front"] {
			r.Candidate = id
		}
	}
	r.Action = "accept"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	r.Action = "collect"
	r.JobID = jobID
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for id, c := range set.Candidates {
		if c.JobID == jobID {
			r.Candidate = id
		}
	}
	r.Action = "accept"
	r.Slot = "body/left"
	r.Revision = set.Revision
	if _, e = s.Reference(ctx, r); e == nil {
		t.Fatal("stale generated candidate silently accepted")
	}
	r.Reapprove = true
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if set.Candidates[r.Candidate].Parents["body/front"] == set.Accepted["body/front"] {
		t.Fatal("reapproval erased original frozen parent lineage")
	}
	req.Key = "cancel"
	req.Revision = set.Revision
	set, e = s.Reference(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CancelJob(ctx, JobRequest{p, set.Jobs["cancel"]}); e != nil {
		t.Fatal(e)
	}
	r.Action = "collect"
	r.JobID = set.Jobs["cancel"]
	r.Revision = set.Revision
	if _, e = s.Reference(ctx, r); e == nil {
		t.Fatal("cancelled job collected")
	}
}
