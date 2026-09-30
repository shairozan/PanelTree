package app

import (
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func generationFixture(t *testing.T) (*adapters.ComfyUI, *atomic.Int32) {
	t.Helper()
	var posts atomic.Int32
	var width, height int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/object_info":
			_, _ = w.Write([]byte(`{"Fake":{},"SaveImage":{}}`))
		case "/prompt":
			posts.Add(1)
			var body struct {
				Prompt map[string]adapters.ComfyNode `json:"prompt"`
			}
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Error(e)
			}
			if body.Prompt["gen"].Inputs["prompt"] == "backend failure" {
				http.Error(w, "unavailable", http.StatusBadGateway)
				return
			}
			wv, _ := body.Prompt["gen"].Inputs["width"].(json.Number).Int64()
			hv, _ := body.Prompt["gen"].Inputs["height"].(json.Number).Int64()
			width, height = int(wv), int(hv)
			_, _ = w.Write([]byte(`{"prompt_id":"id"}`))
		case "/history/id":
			_, _ = w.Write([]byte(`{"id":{"status":{"completed":true,"status_str":"success"},"outputs":{"out":{"images":[{"filename":"a.png","type":"output"}]}}}}`))
		case "/view":
			im := image.NewNRGBA(image.Rect(0, 0, width, height))
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					im.Set(x, y, color.NRGBA{R: 200, A: 255})
				}
			}
			if e := png.Encode(w, im); e != nil {
				t.Error(e)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	p := adapters.ComfyProfile{Version: "comfy-profile/v1", Name: "fixture", Revision: "1", BackendIdentity: "fixture-backend", Models: map[string]string{"checkpoint": "sha256:fixture"}, OutputNode: "out", Workflow: map[string]adapters.ComfyNode{"out": {ClassType: "SaveImage", Inputs: map[string]any{}}, "gen": {ClassType: "Fake", Inputs: map[string]any{"prompt": "", "seed": 0, "width": 64, "height": 64}}}, Bindings: map[string]adapters.ComfyBinding{"prompt": {Node: "gen", Input: "prompt"}, "seed": {Node: "gen", Input: "seed"}, "width": {Node: "gen", Input: "width"}, "height": {Node: "gen", Input: "height"}}}
	return &adapters.ComfyUI{URL: srv.URL, Profile: p}, &posts
}

func TestGeneratedCandidateWorkflow(t *testing.T) {
	_, p, i := editFixture(t)
	c, posts := generationFixture(t)
	s := NewService(WithComfyUI(c))
	ctx := context.Background()
	before, e := os.ReadFile(filepath.Join(filepath.Dir(p), "pages", "01.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	r := AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "generate", Renderer: "comfyui", Generation: &render.Generation{Prompt: "a red hero", Seed: 17}, Width: 120, Height: 180}
	j, e := s.RequestAsset(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.RequestAsset(ctx, r)
	if e != nil || duplicate.ID != j.ID {
		t.Fatalf("duplicate: %+v %v", duplicate, e)
	}
	r.Generation.Seed++
	if _, e = s.RequestAsset(ctx, r); e == nil {
		t.Fatal("changed seed accepted with same key")
	}
	r.Generation.Seed--
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	j, e = s.Job(ctx, JobRequest{p, j.ID})
	if e != nil || j.State != "succeeded" {
		t.Fatalf("generation %+v %v", j, e)
	}
	statusJSON, _ := json.Marshal(j)
	var status map[string]any
	if e = json.Unmarshal(statusJSON, &status); e != nil {
		t.Fatal(e)
	}
	if status["generation_provenance"] == nil {
		t.Fatal("job status omits generation provenance")
	}
	if posts.Load() != 1 {
		t.Fatal("duplicate backend submission")
	}
	store, e := jobs.Open(filepath.Join(filepath.Dir(p), ".paneltree", "jobs"))
	if e != nil {
		t.Fatal(e)
	}
	_, payload, _, e := store.Result(ctx, j.ID)
	if e != nil {
		t.Fatal(e)
	}
	var frozen map[string]any
	if e = json.Unmarshal(payload, &frozen); e != nil {
		t.Fatal(e)
	}
	if frozen["generation_recipe"] == nil {
		t.Fatal("missing persisted generation provenance")
	}
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	if len(view.Layers) != 0 || view.Revision != i.Revision {
		t.Fatal("generation changed selection")
	}
	selected, e := s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision})
	if e != nil {
		t.Fatal(e)
	}
	if selected.Layers[targetKey(target())].Pin == "" {
		t.Fatal("no selected candidate")
	}
	after, e := os.ReadFile(filepath.Join(filepath.Dir(p), "pages", "01.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if string(before) != string(after) {
		t.Fatal("generation modified page schema")
	}
	if _, e = s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "page-01", Width: 120, Height: 180, Output: filepath.Join(filepath.Dir(p), "generated.png")}); e != nil {
		t.Fatal(e)
	}
}

func TestGeneratedCandidatesPreserveProtectedSelections(t *testing.T) {
	for _, protection := range []string{"approve", "lock", "override"} {
		t.Run(protection, func(t *testing.T) {
			_, p, i := editFixture(t)
			c, _ := generationFixture(t)
			s := NewService(WithComfyUI(c))
			ctx := context.Background()
			op := Operation{Target: target(), Action: protection}
			if protection == "lock" {
				op.Scope = model.AssetLock
			}
			if protection == "override" || protection == "approve" {
				op.Artifact = filepath.Join(filepath.Dir(p), "assets", "setting.png")
			}
			revision := i.Revision
			if protection == "approve" {
				revision = applyOp(t, s, p, revision, Operation{Target: target(), Action: "review"}).Revision
			}
			protected := applyOp(t, s, p, revision, op)
			j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: protected.Revision, Target: target(), IdempotencyKey: "protected", Renderer: "comfyui", Generation: &render.Generation{Prompt: "replacement", Seed: 0}, Width: 120, Height: 180})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
				t.Fatal(e)
			}
			status, e := s.Job(ctx, JobRequest{p, j.ID})
			if e != nil || status.State != "succeeded" {
				t.Fatalf("protected draft failed: %+v %v", status, e)
			}
			if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, protected.Revision}); e == nil {
				t.Fatal("protected selection replaced")
			}
			view, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
			if e != nil {
				t.Fatal(e)
			}
			a, _ := json.Marshal(protected.Layers)
			b, _ := json.Marshal(view.Layers)
			if string(a) != string(b) {
				t.Fatal("generation changed protected selection")
			}
		})
	}
}

func TestGeneratedCandidateRejectsChangedProfile(t *testing.T) {
	_, p, i := editFixture(t)
	c, _ := generationFixture(t)
	s := NewService(WithComfyUI(c))
	ctx := context.Background()
	j, e := s.RequestAsset(ctx, AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "profile", Renderer: "comfyui", Generation: &render.Generation{Prompt: "hero"}, Width: 120, Height: 180})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	c.Profile.Models["checkpoint"] = "sha256:new"
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision}); e == nil {
		t.Fatal("model change did not invalidate candidate")
	}
}

func TestGeneratedFailureAndCancellationRetriesDoNotResubmit(t *testing.T) {
	for _, mode := range []string{"failed", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			_, p, i := editFixture(t)
			c, posts := generationFixture(t)
			s := NewService(WithComfyUI(c))
			ctx := context.Background()
			r := AssetRequest{ProjectFile: p, ExpectedRevision: i.Revision, Target: target(), IdempotencyKey: "retry", Renderer: "comfyui", Generation: &render.Generation{Prompt: "backend failure", Seed: 19}, Width: 120, Height: 180}
			j, e := s.RequestAsset(ctx, r)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "cancelled" {
				if _, e = s.CancelJob(ctx, JobRequest{p, j.ID}); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
				t.Fatal(e)
			}
			again, e := s.RequestAsset(ctx, r)
			if e != nil || again.ID != j.ID || string(again.State) != mode {
				t.Fatalf("terminal retry: %+v %v", again, e)
			}
			if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
				t.Fatal(e)
			}
			want := int32(1)
			if mode == "cancelled" {
				want = 0
			}
			if posts.Load() != want {
				t.Fatalf("submissions %d want %d", posts.Load(), want)
			}
			if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision}); e == nil {
				t.Fatal("unsuccessful candidate selected")
			}
		})
	}
}
