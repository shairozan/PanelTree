package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIdeogramCandidateLifecycle(t *testing.T) {
	_, path, view := editFixture(t)
	t.Setenv("IDEOGRAM_API_KEY", "test-key")
	cfg := render.GenerationConfig{DefaultProfile: "story", Profiles: map[string]render.GenerationProfile{"story": {Renderer: "ideogram", Model: "ideogram-3", Operation: "generate", Speed: "quality", MagicPrompt: "off", StyleType: "auto"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	posts := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/image/generate/ideogram-3":
			posts++
			_, _ = w.Write([]byte(`{"generation_id":"remote"}`))
		case "/v2/generations/remote":
			_ = json.NewEncoder(w).Encode(map[string]any{"generation_id": "remote", "status": "completed", "data": []any{map[string]any{"is_image_safe": true, "url": server.URL + "/image"}}})
		case "/image":
			_ = png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s.ideogram.BaseURL = server.URL
	s.ideogram.Client = server.Client()
	ctx := context.Background()
	req := AssetRequest{ProjectFile: path, ExpectedRevision: view.Revision, Target: target(), IdempotencyKey: "ideogram", Renderer: "ideogram", Generation: &render.Generation{Prompt: "castle", Seed: 42}, Width: 120, Height: 180}
	job, e := s.RequestAsset(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.RequestAsset(ctx, req)
	if e != nil || duplicate.ID != job.ID {
		t.Fatalf("duplicate %v", e)
	}
	changed := cfg.Profiles["story"]
	changed.Speed = "turbo"
	s.generation.Profiles["story"] = changed
	if _, e = s.RunJobs(ctx, RunJobsRequest{path, 1}); e != nil {
		t.Fatal(e)
	}
	job, e = s.Job(ctx, JobRequest{path, job.ID})
	if e != nil || job.State != "succeeded" {
		t.Fatalf("job %+v: %v", job, e)
	}
	if job.GenerationProvenance == nil || job.GenerationProvenance.Adapter != "ideogram/v1" {
		t.Fatal("missing provenance")
	}
	s = NewService()
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{ProjectFile: path, JobID: job.ID, ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
	var execution map[string]any
	if e = json.Unmarshal(job.Execution, &execution); e != nil {
		t.Fatal(e)
	}
	if execution["result"] == nil {
		t.Fatal("provider result metadata was not retained")
	}
	if posts != 1 {
		t.Fatalf("posts %d", posts)
	}
}
func TestIdeogramReferenceRequest(t *testing.T) {
	_, path, _ := editFixture(t)
	t.Setenv("IDEOGRAM_API_KEY", "test-key")
	cfg := render.GenerationConfig{DefaultProfile: "story", Profiles: map[string]render.GenerationProfile{"story": {Renderer: "ideogram", Model: "ideogram-3", Operation: "generate", Speed: "quality", MagicPrompt: "off", StyleType: "auto"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	r := ReferenceRequest{ProjectFile: path, Set: "hero", Action: "create"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Action = "request"
	r.Revision = set.Revision
	r.Slot = "body/front"
	r.Key = "front"
	r.Width = 512
	r.Height = 512
	r.Generation = &render.Generation{Prompt: "wizard"}
	r.License = "test"
	r.Attribution = "fixture"
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if set.Jobs["front"] == "" || len(set.Accepted) != 0 {
		t.Fatal("missing unapproved generation")
	}
}
func TestComfyRejectsIdeogramReferenceOptions(t *testing.T) {
	_, path, view := editFixture(t)
	c, _ := generationFixture(t)
	s := NewService(WithComfyUI(c))
	_, e := s.RequestAsset(context.Background(), AssetRequest{ProjectFile: path, ExpectedRevision: view.Revision, Target: target(), IdempotencyKey: "unsupported-image", Renderer: "comfyui", Generation: &render.Generation{Prompt: "hero", CharacterReference: "hero.png"}, Width: 120, Height: 180})
	if e == nil {
		t.Fatal("ComfyUI silently ignored the character-image option")
	}
}
func TestIdeogramDiscoveryReportsImageRoles(t *testing.T) {
	t.Setenv("IDEOGRAM_API_KEY", "key")
	cfg := render.GenerationConfig{Profiles: map[string]render.GenerationProfile{"character": {Renderer: "ideogram", Model: "ideogram-3", Operation: "character", Speed: "quality", MagicPrompt: "off", StyleType: "auto"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	for _, cap := range s.Renderers() {
		if cap.Name == "ideogram" {
			if cap.ImageConditioning == nil {
				t.Fatal("image capabilities missing")
			}
			return
		}
	}
	t.Fatal("ideogram missing")
}

func TestIdeogram45CandidateLifecycle(t *testing.T) {
	_, path, view := editFixture(t)
	t.Setenv("IDEOGRAM_API_KEY", "test-key")
	cfg := render.GenerationConfig{DefaultProfile: "story", Profiles: map[string]render.GenerationProfile{"story": {Renderer: "ideogram", Model: "ideogram-4-5", Operation: "generate", MagicPrompt: "off"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	posts := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/image/generate/ideogram-4-5":
			posts++
			_, _ = w.Write([]byte(`{"generation_id":"remote"}`))
		case "/v2/generations/remote":
			_ = json.NewEncoder(w).Encode(map[string]any{"generation_id": "remote", "status": "completed", "data": []any{map[string]any{"is_image_safe": true, "url": server.URL + "/image"}}})
		case "/image":
			_ = png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s.ideogram.BaseURL = server.URL
	s.ideogram.Client = server.Client()
	ctx := context.Background()
	req := AssetRequest{ProjectFile: path, ExpectedRevision: view.Revision, Target: target(), IdempotencyKey: "ideogram", Renderer: "ideogram", Generation: &render.Generation{Prompt: "castle", Seed: 42}, Width: 120, Height: 180}
	job, e := s.RequestAsset(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.RequestAsset(ctx, req)
	if e != nil || duplicate.ID != job.ID {
		t.Fatalf("duplicate %v", e)
	}
	changed := cfg.Profiles["story"]
	changed.Speed = "turbo"
	s.generation.Profiles["story"] = changed
	if _, e = s.RunJobs(ctx, RunJobsRequest{path, 1}); e != nil {
		t.Fatal(e)
	}
	job, e = s.Job(ctx, JobRequest{path, job.ID})
	if e != nil || job.State != "succeeded" {
		t.Fatalf("job %+v: %v", job, e)
	}
	if job.GenerationProvenance == nil || job.GenerationProvenance.Adapter != "ideogram/v1" {
		t.Fatal("missing provenance")
	}
	s = NewService()
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{ProjectFile: path, JobID: job.ID, ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
	var execution map[string]any
	if e = json.Unmarshal(job.Execution, &execution); e != nil {
		t.Fatal(e)
	}
	if execution["result"] == nil {
		t.Fatal("provider result metadata was not retained")
	}
	if posts != 1 {
		t.Fatalf("posts %d", posts)
	}
}

func TestIdeogram45Discovery(t *testing.T) {
	cfg := render.GenerationConfig{Profiles: map[string]render.GenerationProfile{"edit": {Renderer: "ideogram", Model: "ideogram-4-5", Operation: "edit", MagicPrompt: "off"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	for _, cap := range s.Renderers() {
		if cap.Name == "ideogram" {
			if cap.ImageConditioning.Roles["style"] != 4 || cap.ImageConditioning.RequiredRoles["character"] != 1 {
				t.Fatal("wrong 4.5 limits")
			}
			b, _ := json.Marshal(cap)
			var fields map[string]any
			_ = json.Unmarshal(b, &fields)
			if fields["attribution"] != "Powered by Ideogram" || fields["usage_policy"] != "https://ideogram.ai/legal/usage-policy/" {
				t.Fatal("missing provider attribution/policy")
			}
			return
		}
	}
	t.Fatal("missing renderer")
}

func TestIdeogram45EditCandidateLifecycle(t *testing.T) {
	_, path, view := editFixture(t)
	t.Setenv("IDEOGRAM_API_KEY", "test-key")
	cfg := render.GenerationConfig{DefaultProfile: "story", Profiles: map[string]render.GenerationProfile{"story": {Renderer: "ideogram", Model: "ideogram-4-5", Operation: "edit", MagicPrompt: "off", Quality: "high", Size: "source"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	posts := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/image/generate/ideogram-4-5":
			posts++
			if e := r.ParseMultipartForm(1 << 20); e != nil {
				t.Error(e)
				return
			}
			defer func() { _ = r.MultipartForm.RemoveAll() }()
			if r.FormValue("quality") != "high" || r.FormValue("size") != "source" || len(r.MultipartForm.File["images"]) != 1 {
				t.Error("edit recipe not preserved")
			}
			_, _ = w.Write([]byte(`{"generation_id":"remote"}`))
		case "/v2/generations/remote":
			_ = json.NewEncoder(w).Encode(map[string]any{"generation_id": "remote", "status": "completed", "data": []any{map[string]any{"is_image_safe": true, "url": server.URL + "/image"}}})
		case "/image":
			_ = png.Encode(w, image.NewNRGBA(image.Rect(0, 0, 1024, 1024)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s.ideogram.BaseURL = server.URL
	s.ideogram.Client = server.Client()
	var source bytes.Buffer
	if e := png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 64, 96))); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(filepath.Dir(path), "source.png"), source.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	req := AssetRequest{ProjectFile: path, ExpectedRevision: view.Revision, Target: target(), IdempotencyKey: "ideogram", Renderer: "ideogram", Generation: &render.Generation{Prompt: "laughing and pointing", Seed: 42, CharacterReference: "source.png"}, Width: 120, Height: 180}
	job, e := s.RequestAsset(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.RequestAsset(ctx, req)
	if e != nil || duplicate.ID != job.ID {
		t.Fatalf("duplicate %v", e)
	}
	changed := cfg.Profiles["story"]
	changed.Speed = "turbo"
	s.generation.Profiles["story"] = changed
	if _, e = s.RunJobs(ctx, RunJobsRequest{path, 1}); e != nil {
		t.Fatal(e)
	}
	job, e = s.Job(ctx, JobRequest{path, job.ID})
	if e != nil || job.State != "succeeded" {
		t.Fatalf("job %+v: %v", job, e)
	}
	if job.GenerationProvenance == nil || job.GenerationProvenance.Adapter != "ideogram/v1" {
		t.Fatal("missing provenance")
	}
	s = NewService()
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{ProjectFile: path, JobID: job.ID, ExpectedRevision: view.Revision}); e != nil {
		t.Fatal(e)
	}
	var execution map[string]any
	if e = json.Unmarshal(job.Execution, &execution); e != nil {
		t.Fatal(e)
	}
	if execution["result"] == nil {
		t.Fatal("provider result metadata was not retained")
	}
	if posts != 1 {
		t.Fatalf("posts %d", posts)
	}
}
