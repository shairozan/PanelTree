package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
)

// Explicitly opt in: three real generations, preserving inputs and output for
// visual evaluation. Ordinary CI never needs a GPU or model download.
func TestCharacterRealBackendEvaluation(t *testing.T) {
	destination := os.Getenv("PANELTREE_CHARACTER_SMOKE_DIR")
	if destination == "" {
		t.Skip("opt-in multi-panel character evaluation")
	}
	endpoint, profile := os.Getenv("PANELTREE_COMFY_URL"), os.Getenv("PANELTREE_COMFY_PROFILE")
	if endpoint == "" || profile == "" {
		t.Fatal("set PANELTREE_COMFY_URL and PANELTREE_COMFY_PROFILE")
	}
	if _, e := os.Stat(destination); !os.IsNotExist(e) {
		t.Fatal("evaluation destination must not exist")
	}
	if e := os.CopyFS(destination, os.DirFS("../examples/characters")); e != nil {
		t.Fatal(e)
	}
	s, e := NewRuntimeService(endpoint, profile)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	defer cancel()
	p := filepath.Join(destination, "project.yaml")
	for _, panel := range []string{"front", "turn", "side"} {
		view, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
		if e != nil {
			t.Fatal(e)
		}
		req := AssetRequest{ProjectFile: p, ExpectedRevision: view.Revision, IdempotencyKey: "character-" + panel, Renderer: "comfyui", Target: LayerTarget{Page: "character-evaluation", Panel: model.ID(panel), Layer: "alex"}}
		req.Width, req.Height = 1536, 512
		req.Generation = &render.Generation{Prompt: "comic book illustration, one person, full body, standing on a plain pale background", Seed: 17, NegativePrompt: "text, watermark, blurry"}
		j, e := s.RequestAsset(ctx, req)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
			t.Fatal(e)
		}
		j, e = s.Job(ctx, JobRequest{p, j.ID})
		if e != nil || j.State != "succeeded" {
			t.Fatalf("generation: %+v %v", j, e)
		}
		if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, view.Revision}); e != nil {
			t.Fatal(e)
		}
		t.Logf("panel=%s job=%s provenance=%+v", panel, j.ID, j.GenerationProvenance)
	}
	built, e := s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "character-evaluation", Width: 1536, Height: 512, Output: filepath.Join(destination, "character-evaluation.png")})
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("Visually evaluate %s using docs/characters.md", built.Output)
}
