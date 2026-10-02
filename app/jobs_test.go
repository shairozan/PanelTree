package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/model"
	"os"
	"path/filepath"
	"testing"
)

func TestJobRendererOnlyReadsFrozenFiles(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	_, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "original", Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	store, e := jobs.Open(filepath.Join(filepath.Dir(p), ".paneltree", "jobs"))
	if e != nil {
		t.Fatal(e)
	}
	_, data, e := store.Lookup(ctx, "original")
	if e != nil {
		t.Fatal(e)
	}
	var input assetInput
	if e = json.Unmarshal(data, &input); e != nil {
		t.Fatal(e)
	}
	input.Request.Source.Path = filepath.Join(filepath.Dir(p), "assets", "hero.png")
	input.Files = nil
	payload, e := json.Marshal(input)
	if e != nil {
		t.Fatal(e)
	}
	injected, e := store.Submit(ctx, "outside-frozen", i.Revision, payload)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{ProjectFile: p, Workers: 1}); e != nil {
		t.Fatal(e)
	}
	j, e := s.Job(ctx, JobRequest{ProjectFile: p, ID: injected.ID})
	if e != nil {
		t.Fatal(e)
	}
	if j.State != "failed" {
		t.Fatalf("renderer read unfrozen file: %+v", j)
	}
}

func TestAssetJobStaticCapabilities(t *testing.T) {
	for _, layer := range []string{"hero", "lantern", "lettering"} {
		t.Run(layer, func(t *testing.T) {
			s, p, i := editFixture(t)
			tar := target()
			tar.Layer = model.ID(layer)
			ctx := context.Background()
			j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: tar, IdempotencyKey: layer, Width: 120, Height: 180})
			if e != nil {
				t.Fatal(e)
			}
			if j.State != "queued" || j.Revision != i.Revision {
				t.Fatalf("invalid submission %+v", j)
			}
			if _, e = s.RunJobs(ctx, RunJobsRequest{p, 2}); e != nil {
				t.Fatal(e)
			}
			j, e = s.Job(ctx, JobRequest{p, j.ID})
			if e != nil || j.State != "succeeded" {
				t.Fatalf("static job %+v %v", j, e)
			}
			r, e := s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision})
			if e != nil {
				t.Fatal(e)
			}
			st := r.Layers[targetKey(tar)]
			if st.State != model.Draft || st.Pin == "" || st.Manual != "" {
				t.Fatalf("invalid draft selection: %+v", st)
			}
		})
	}
}
func TestAssetJobRevisionAndInputSnapshot(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	r := AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "snapshot", Width: 120, Height: 180}
	j, e := s.RequestAsset(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.RequestAsset(ctx, r)
	if e != nil || again.ID != j.ID {
		t.Fatalf("duplicate submit: %v", e)
	}
	d := i.Documents[0].Document
	d.Book.Title = "new revision"
	edited, e := s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "project.yaml", Document: d}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "assets", "hero.png"), []byte("source changed after submission"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	done, e := s.Job(ctx, JobRequest{p, j.ID})
	if e != nil || done.State != "succeeded" || done.Revision != i.Revision {
		t.Fatalf("did not use frozen input: %+v %v", done, e)
	}
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, edited.Revision}); e == nil {
		t.Fatal("stale candidate selected")
	}
	if len(edited.Layers) != 0 {
		t.Fatal("submission or execution changed selections")
	}
}
func TestUnavailableRendererStructuredError(t *testing.T) {
	s, p, i := editFixture(t)
	caps := s.Renderers()
	if len(caps) == 0 {
		t.Fatal("renderer discovery empty")
	}
	_, e := s.RequestAsset(context.Background(), AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "unsupported", Renderer: "comfyui"})
	var d *JobDiagnostic
	if !errors.As(e, &d) || d.Code != "renderer_unavailable" {
		t.Fatalf("unstructured unavailable renderer: %v", e)
	}
}
func TestCandidateSelectionRespectsLocks(t *testing.T) {
	s, p, i := editFixture(t)
	r := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "lock", Scope: model.AssetLock})
	ctx := context.Background()
	j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: r.Revision, Target: target(), IdempotencyKey: "locked", Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, r.Revision}); e == nil {
		t.Fatal("candidate selection bypassed lock")
	}
}

func TestAssetJobRetryAfterEdits(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	r := AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "retry", Width: 120, Height: 180}
	j, e := s.RequestAsset(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	d := i.Documents[0].Document
	d.Book.Title = "edited after submit"
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "project.yaml", Document: d}}}); e != nil {
		t.Fatal(e)
	}
	same, e := s.RequestAsset(ctx, r)
	if e != nil || same.ID != j.ID {
		t.Fatalf("idempotent retry lost original job: %+v %v", same, e)
	}
	r.Target.Layer = "lantern"
	if _, e = s.RequestAsset(ctx, r); e == nil {
		t.Fatal("retry key accepted a different target")
	}
}
func TestCandidateRejectsChangedDependency(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "dependency", Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(filepath.Dir(p), "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "assets", "hero.png"), data, 0600); e != nil {
		t.Fatal(e)
	}
	_, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision})
	var d *JobDiagnostic
	if !errors.As(e, &d) || d.Code != "stale_candidate" {
		t.Fatalf("changed dependency did not reject stale candidate: %v", e)
	}
}

func TestRunJobRequiresID(t *testing.T) {
	s, p, i := editFixture(t)
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: key, Width: 120, Height: 180}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.RunJob(ctx, JobRequest{ProjectFile: p}); e == nil {
		t.Error("missing ID accepted")
	}
	js, e := s.Jobs(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	for _, j := range js {
		if j.State != "queued" {
			t.Errorf("unconfirmed job executed: %s", j.State)
		}
	}
}
