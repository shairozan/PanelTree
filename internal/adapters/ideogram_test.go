package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	assets "github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func ideogramProfile() render.GenerationProfile {
	return render.GenerationProfile{Renderer: "ideogram", Model: "ideogram-3", Operation: "generate", Speed: "quality", MagicPrompt: "off", StyleType: "auto"}
}
func TestIdeogramSubmitPollAndResume(t *testing.T) {
	r, e := FreezeIdeogram("test", ideogramProfile(), render.Request{Generation: &render.Generation{Prompt: "a castle", Seed: 42}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 512}}})
	if e != nil {
		t.Fatal(e)
	}
	// Fixture encoding is setup, not part of the provider request deadline.
	var fixture bytes.Buffer
	if e := png.Encode(&fixture, image.NewNRGBA(image.Rect(0, 0, 1024, 1024))); e != nil {
		t.Fatal(e)
	}
	posts := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v2/image/generate/ideogram-3":
			posts++
			if req.Header.Get("Api-Key") != "test-key" {
				t.Error("missing key")
			}
			if e := req.ParseMultipartForm(1 << 20); e != nil {
				t.Error(e)
				return
			}
			if req.FormValue("prompt") != "a castle" || req.FormValue("async") != "true" || req.FormValue("seed") != "42" {
				t.Error("wrong submitted recipe")
			}
			_, _ = w.Write([]byte(`{"generation_id":"remote-id"}`))
		case "/v2/generations/remote-id":
			_ = json.NewEncoder(w).Encode(map[string]any{"generation_id": "remote-id", "status": "completed", "data": []any{map[string]any{"is_image_safe": true, "url": srv.URL + "/image", "seed": 42}}})
		case "/image":
			if req.Header.Get("Api-Key") != "" {
				t.Error("key leaked to image download")
			}
			_, _ = w.Write(fixture.Bytes())
		default:
			t.Errorf("unexpected path %s", req.URL.Path)
			http.NotFound(w, req)
		}
	}))
	defer srv.Close()
	c := &Ideogram{APIKey: "test-key", BaseURL: srv.URL, Client: srv.Client(), Timeout: 10 * time.Second, PollInterval: time.Millisecond}
	var state IdeogramState
	save := func(v IdeogramState) error { state = v; return nil }
	data, e := c.Run(context.Background(), r, state, save)
	if e != nil || len(data) == 0 {
		t.Fatalf("generate: %v", e)
	}
	if _, e = c.Run(context.Background(), r, state, save); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatalf("resume resubmitted paid request: %d", posts)
	}
	if _, e = c.Run(context.Background(), r, IdeogramState{Started: true}, save); e == nil {
		t.Fatal("ambiguous submission must not retry")
	}
}
func TestIdeogramFreezeRejectsUnsupportedInputs(t *testing.T) {
	p := ideogramProfile()
	p.Operation = "character"
	_, e := FreezeIdeogram("test", p, render.Request{Generation: &render.Generation{Prompt: "person"}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 512}}})
	if e == nil {
		t.Fatal("missing character accepted")
	}
}

func TestIdeogramEstimateDoesNotGenerate(t *testing.T) {
	r, e := FreezeIdeogram("test", ideogramProfile(), render.Request{Generation: &render.Generation{Prompt: "castle"}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 512}}})
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("dry_run") != "true" {
			t.Fatal("estimate would generate")
		}
		_, _ = w.Write([]byte(`{"provider_quote":"opaque"}`))
	}))
	defer srv.Close()
	c := &Ideogram{APIKey: "key", BaseURL: srv.URL, Client: srv.Client()}
	data, e := c.Estimate(context.Background(), r)
	if e != nil || string(data) != `{"provider_quote":"opaque"}` || calls != 1 {
		t.Fatalf("quote: %s %v", data, e)
	}
}

func TestIdeogramCharacterAndStyleTransport(t *testing.T) {
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8, 8))); e != nil {
		t.Fatal(e)
	}
	data := b.Bytes()
	inputs := []render.ImageInput{{Role: "character", SHA256: assets.Digest(data), PNG: data}, {Role: "style", SHA256: assets.Digest(data), PNG: data}}
	p := ideogramProfile()
	p.Operation = "character"
	req := render.Request{Generation: &render.Generation{Prompt: "profile view"}, ImageInputs: inputs, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 768}}}
	recipe, e := FreezeIdeogram("character", p, req)
	if e != nil {
		t.Fatal(e)
	}
	if recipe.AspectRatio != "2x3" {
		t.Fatalf("aspect %s", recipe.AspectRatio)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/image/generate/ideogram-3-character" || r.URL.Query().Get("dry_run") != "true" {
			t.Error("incorrect endpoint")
		}
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			t.Error(e)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		for _, role := range []string{"character_reference_images", "style_reference_images"} {
			if len(r.MultipartForm.File[role]) != 1 {
				t.Errorf("missing %s", role)
			}
		}
		_, _ = w.Write([]byte(`{"quote":"test"}`))
	}))
	defer srv.Close()
	c := &Ideogram{APIKey: "secret", BaseURL: srv.URL, Client: srv.Client()}
	if _, e = c.Estimate(context.Background(), recipe); e != nil {
		t.Fatal(e)
	}
	inputs[0].PNG[0] = 0
	if recipe.Images[0].PNG[0] == 0 {
		t.Fatal("reference bytes were not frozen")
	}
}
func TestIdeogramFailuresDoNotResubmit(t *testing.T) {
	for _, status := range []int{401, 402, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts++
				w.WriteHeader(status)
				_, _ = w.Write([]byte("sensitive-provider-body"))
			}))
			defer srv.Close()
			c := &Ideogram{APIKey: "secret", BaseURL: srv.URL, Client: srv.Client()}
			recipe, e := FreezeIdeogram("test", ideogramProfile(), render.Request{Generation: &render.Generation{Prompt: "castle"}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 512}}})
			if e != nil {
				t.Fatal(e)
			}
			var state IdeogramState
			save := func(v IdeogramState) error { state = v; return nil }
			if _, e = c.Run(context.Background(), recipe, state, save); e == nil || strings.Contains(e.Error(), "sensitive") {
				t.Fatalf("failure: %v", e)
			}
			if _, e = c.Run(context.Background(), recipe, state, save); e == nil {
				t.Fatal("ambiguous retry permitted")
			}
			if posts != 1 {
				t.Fatal("paid request retried")
			}
		})
	}
}
func TestIdeogramPendingCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"generation_id":"remote","status":"pending"}`))
	}))
	defer srv.Close()
	c := &Ideogram{APIKey: "key", BaseURL: srv.URL, Client: srv.Client(), Timeout: 20 * time.Millisecond, PollInterval: time.Second}
	recipe, e := FreezeIdeogram("test", ideogramProfile(), render.Request{Generation: &render.Generation{Prompt: "castle"}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 512}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Run(context.Background(), recipe, IdeogramState{Started: true, GenerationID: "remote"}, func(IdeogramState) error { return nil }); e == nil {
		t.Fatal("pending job did not time out")
	}
}
