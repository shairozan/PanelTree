package app

import (
	"context"
	"github.com/shairozan/PanelTree/render"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in only. This submits real compute work and retains a reviewable book.
func TestComfyRealBackendSmoke(t *testing.T) {
	endpoint, profile, destination := os.Getenv("PANELTREE_COMFY_URL"), os.Getenv("PANELTREE_COMFY_PROFILE"), os.Getenv("PANELTREE_COMFY_SMOKE_DIR")
	if endpoint == "" && profile == "" && destination == "" {
		t.Skip("opt-in real ComfyUI smoke test")
	}
	if endpoint == "" || profile == "" || destination == "" {
		t.Fatal("set PANELTREE_COMFY_URL, PANELTREE_COMFY_PROFILE and a new PANELTREE_COMFY_SMOKE_DIR")
	}
	s, e := NewRuntimeService(endpoint, profile)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	initialized, e := s.Init(ctx, InitRequest{Directory: destination})
	if e != nil {
		t.Fatal(e)
	}
	p := initialized.ProjectFile
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: view.Revision, Target: LayerTarget{Page: "page-01", Panel: "p1", Layer: "setting"}, Renderer: "comfyui", IdempotencyKey: "real-smoke-1", Generation: &render.Generation{Prompt: "moonlit coastal city, comic book background, no people", Seed: 17}, Width: 800, Height: 1200})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	j, e = s.Job(ctx, JobRequest{p, j.ID})
	if e != nil || j.State != "succeeded" {
		t.Fatalf("real backend: %+v %v", j, e)
	}
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, view.Revision}); e != nil {
		t.Fatal(e)
	}
	built, e := s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "page-01", Width: 800, Height: 1200, Output: filepath.Join(destination, "comfy-smoke.png")})
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("Review %s; provenance %+v", built.Output, j.GenerationProvenance)
}
