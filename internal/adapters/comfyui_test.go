package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func comfyProfile() ComfyProfile {
	return ComfyProfile{Version: "comfy-profile/v1", Name: "fixture", Revision: "1", BackendIdentity: "test-build", Models: map[string]string{"checkpoint": "sha256:abc"}, OutputNode: "out", Workflow: map[string]ComfyNode{"out": {ClassType: "SaveImage", Inputs: map[string]any{}}, "gen": {ClassType: "Fake", Inputs: map[string]any{"prompt": "", "seed": 0, "width": 64, "height": 64}}}, Bindings: map[string]ComfyBinding{"prompt": {"gen", "prompt"}, "seed": {"gen", "seed"}, "width": {"gen", "width"}, "height": {"gen", "height"}}}
}
func comfyRequest() render.Request {
	return render.Request{Generation: &render.Generation{Prompt: "a lantern", Seed: 9007199254740993}, Scene: scene.Context{PixelSize: model.Canvas{Width: 63, Height: 61}}}
}

func TestComfyFreeze(t *testing.T) {
	c := &ComfyUI{Profile: comfyProfile()}
	a, e := c.Freeze(comfyRequest())
	if e != nil {
		t.Fatal(e)
	}
	if a.Width != 64 || a.Height != 64 || a.Workflow["gen"].Inputs["prompt"] != "a lantern" || a.Workflow["gen"].Inputs["seed"] != uint64(9007199254740993) {
		t.Fatalf("incorrect semantic translation: %+v", a)
	}
	c.Profile.Models["checkpoint"] = "sha256:changed"
	b, e := c.Freeze(comfyRequest())
	if e != nil {
		t.Fatal(e)
	}
	if a.ProfileHash == b.ProfileHash || a.Profile.Models["checkpoint"] != "sha256:abc" {
		t.Fatal("model identity not frozen/hashed")
	}
	c.Profile.Workflow["gen"].Inputs["steps"] = 30
	d, e := c.Freeze(comfyRequest())
	if e != nil {
		t.Fatal(e)
	}
	if b.ProfileHash == d.ProfileHash {
		t.Fatal("effective workflow not hashed")
	}
}

func TestComfyRejectsUnsupportedRequests(t *testing.T) {
	for _, name := range []string{"rgba", "empty prompt", "bad binding", "missing model", "too large", "negative unsupported"} {
		t.Run(name, func(t *testing.T) {
			c := &ComfyUI{Profile: comfyProfile()}
			r := comfyRequest()
			switch name {
			case "rgba":
				r.Generation.Output = "isolated-rgba"
			case "empty prompt":
				r.Generation.Prompt = ""
			case "bad binding":
				c.Profile.Bindings["seed"] = ComfyBinding{"missing", "seed"}
			case "missing model":
				c.Profile.Models = nil
			case "too large":
				r.Scene.PixelSize.Width = 99999
			case "negative unsupported":
				r.Generation.NegativePrompt = "blur"
			}
			if _, e := c.Freeze(r); e == nil {
				t.Fatal("unsupported request accepted")
			}
		})
	}
}

func TestComfyHTTP(t *testing.T) {
	for _, mode := range []string{"success", "retry history", "timeout", "cancel", "submit error", "execution error", "ambiguous submission", "bad image", "wrong size", "multiple images", "cancel unsupported"} {
		t.Run(mode, func(t *testing.T) {
			var posts, polls, cancels atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/object_info":
					_, _ = w.Write([]byte(`{"Fake":{},"SaveImage":{}}`))
				case r.URL.Path == "/prompt":
					posts.Add(1)
					var body struct {
						Prompt map[string]ComfyNode `json:"prompt"`
					}
					dec := json.NewDecoder(r.Body)
					dec.UseNumber()
					if e := dec.Decode(&body); e != nil {
						t.Error(e)
					}
					if body.Prompt["gen"].Inputs["seed"] != json.Number("9007199254740993") {
						t.Error("seed lost precision")
					}
					if mode == "submit error" {
						http.Error(w, "invalid workflow", http.StatusBadRequest)
						return
					}
					if mode == "ambiguous submission" {
						http.Error(w, "gateway lost response", http.StatusBadGateway)
						return
					}
					_, _ = w.Write([]byte(`{"prompt_id":"test-id"}`))
				case r.URL.Path == "/history/test-id":
					n := polls.Add(1)
					if mode == "cancel" {
						cancel()
					}
					if mode == "retry history" && n == 1 {
						http.Error(w, "temporary", http.StatusServiceUnavailable)
						return
					}
					if mode == "timeout" || mode == "cancel" || mode == "cancel unsupported" {
						_, _ = w.Write([]byte(`{}`))
						return
					}
					if mode == "execution error" {
						_, _ = w.Write([]byte(`{"test-id":{"status":{"completed":false,"status_str":"error"}}}`))
						return
					}
					images := `[{"filename":"result.png","subfolder":"output folder","type":"output"}]`
					if mode == "multiple images" {
						images = `[{"filename":"a.png"},{"filename":"b.png"}]`
					}
					_, _ = w.Write([]byte(`{"test-id":{"status":{"completed":true,"status_str":"success"},"outputs":{"out":{"images":` + images + `}}}}`))
				case r.URL.Path == "/view":
					if r.URL.Query().Get("subfolder") != "output folder" {
						t.Error("artifact query damaged")
					}
					if mode == "bad image" {
						_, _ = w.Write([]byte("not png"))
						return
					}
					size := 64
					if mode == "wrong size" {
						size = 2
					}
					im := image.NewNRGBA(image.Rect(0, 0, size, size))
					im.Set(0, 0, color.NRGBA{R: 100, A: 40})
					if e := png.Encode(w, im); e != nil {
						t.Error(e)
					}
				case strings.HasSuffix(r.URL.Path, "/cancel"):
					if r.URL.Path != "/api/jobs/test-id/cancel" {
						t.Error("wrong cancellation target")
					}
					cancels.Add(1)
					if mode == "cancel unsupported" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(`{"cancelled":true}`))
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			c := &ComfyUI{URL: srv.URL, Profile: comfyProfile(), Timeout: 150 * time.Millisecond, PollInterval: time.Millisecond}
			if mode != "timeout" && mode != "cancel unsupported" {
				c.Timeout = 2 * time.Second
			}
			recipe, e := c.Freeze(comfyRequest())
			if e != nil {
				t.Fatal(e)
			}
			im, e := c.Generate(ctx, recipe)
			switch mode {
			case "success", "retry history":
				if e != nil {
					t.Fatal(e)
				}
				_, _, _, a := im.At(0, 0).RGBA()
				if a != 65535 {
					t.Fatal("RGB profile returned transparency")
				}
			case "timeout", "cancel unsupported":
				if !errors.Is(e, context.DeadlineExceeded) {
					t.Fatalf("expected timeout: %v", e)
				}
			case "cancel":
				if !errors.Is(e, context.Canceled) {
					t.Fatalf("expected cancellation: %v", e)
				}
			default:
				if e == nil {
					t.Fatal("expected backend/artifact error")
				}
			}
			if posts.Load() != 1 {
				t.Fatalf("unsafe submit count %d", posts.Load())
			}
			if mode == "submit error" && !strings.Contains(e.Error(), "invalid workflow") {
				t.Fatalf("backend diagnostic lost: %v", e)
			}
			if (mode == "timeout" || mode == "cancel" || mode == "cancel unsupported") && cancels.Load() != 1 {
				t.Fatal("backend cancellation not attempted")
			}
			if mode == "cancel unsupported" && !strings.Contains(e.Error(), "cancel") {
				t.Fatal("cleanup failure hidden")
			}
			if mode == "retry history" && polls.Load() < 2 {
				t.Fatal("read not retried")
			}
		})
	}
}

func TestComfyRecipeRoundTrip(t *testing.T) {
	c := &ComfyUI{Profile: comfyProfile()}
	a, e := c.Freeze(comfyRequest())
	if e != nil {
		t.Fatal(e)
	}
	data, e := json.Marshal(a)
	if e != nil {
		t.Fatal(e)
	}
	var b ComfyRecipe
	if e = json.Unmarshal(data, &b); e != nil {
		t.Fatal(e)
	}
	again, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("durable recipe lost numeric precision")
	}
}
