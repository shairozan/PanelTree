package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	assets "github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
)

func profile45(t *testing.T, operation, quality, size string) render.GenerationProfile {
	t.Helper()
	var p render.GenerationProfile
	b, _ := json.Marshal(map[string]string{"renderer": "ideogram", "model": "ideogram-4-5", "operation": operation, "quality": quality, "size": size, "magic_prompt": "off"})
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func request45(t *testing.T) render.Request {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 32, 48))); err != nil {
		t.Fatal(err)
	}
	data := bytes.Clone(b.Bytes())
	b.Reset()
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 48, 32))); err != nil {
		t.Fatal(err)
	}
	other := b.Bytes()
	return render.Request{Generation: &render.Generation{Prompt: "Laughing and pointing", Seed: 43}, Scene: scene.Context{PixelSize: model.Canvas{Width: 512, Height: 768}}, ImageInputs: []render.ImageInput{{Role: "character", SHA256: assets.Digest(data), PNG: data}, {Role: "style", SHA256: assets.Digest(other), PNG: other}}}
}
func TestIdeogram45Transport(t *testing.T) {
	req := request45(t)
	recipe, err := FreezeIdeogram("edit", profile45(t, "edit", "high", "source"), req)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.AspectRatio != "" {
		t.Fatal("4.5 must not freeze a 3.0 aspect ratio")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/image/generate/ideogram-4-5" || r.URL.Query().Get("dry_run") != "true" {
			t.Error("incorrect endpoint")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		for key, want := range map[string]string{"quality": "high", "size": "source", "magic_prompt": "off", "async": "true", "num_images": "1", "seed": "43"} {
			if r.FormValue(key) != want {
				t.Errorf("%s=%q", key, r.FormValue(key))
			}
		}
		for _, key := range []string{"negative_prompt", "aspect_ratio", "rendering_speed", "style_type"} {
			if _, ok := r.MultipartForm.Value[key]; ok {
				t.Errorf("unsupported field %s", key)
			}
		}
		files := r.MultipartForm.File["images"]
		if len(files) != 2 {
			t.Errorf("images=%d", len(files))
			return
		}
		for i, file := range files {
			f, e := file.Open()
			if e != nil {
				t.Error(e)
				return
			}
			data, e := io.ReadAll(f)
			_ = f.Close()
			if e != nil || !bytes.Equal(data, req.ImageInputs[i].PNG) {
				t.Error("image order/bytes changed")
			}
		}
		_, _ = w.Write([]byte(`{"object":"price_quote","usd_micros":220000}`))
	}))
	defer srv.Close()
	c := &Ideogram{APIKey: "test", BaseURL: srv.URL, Client: srv.Client()}
	if _, err = c.Estimate(context.Background(), recipe); err != nil {
		t.Fatal(err)
	}
}
func TestIdeogram45Validation(t *testing.T) {
	for _, op := range []string{"generate", "edit"} {
		t.Run(op, func(t *testing.T) {
			req := request45(t)
			size := "source"
			if op == "generate" {
				req.ImageInputs = nil
				size = "auto"
			}
			if _, err := FreezeIdeogram("test", profile45(t, op, "high", size), req); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, name := range []string{"missing-source", "too-many", "negative", "old-speed", "source-without-image", "very-low-without-image", "unknown-size", "unknown-quality", "style-only", "source-not-first"} {
		t.Run(name, func(t *testing.T) {
			req := request45(t)
			p := profile45(t, "edit", "high", "source")
			switch name {
			case "missing-source":
				req.ImageInputs = nil
			case "too-many":
				for len(req.ImageInputs) < 6 {
					req.ImageInputs = append(req.ImageInputs, req.ImageInputs[1])
				}
			case "negative":
				req.Generation.NegativePrompt = "blur"
			case "old-speed":
				p.Speed = "quality"
			case "source-without-image":
				p = profile45(t, "generate", "high", "source")
				req.ImageInputs = nil
			case "very-low-without-image":
				p = profile45(t, "generate", "very_low", "auto")
				req.ImageInputs = nil
			case "unknown-size":
				p = profile45(t, "edit", "high", "nonsense")
			case "unknown-quality":
				p = profile45(t, "edit", "nonsense", "source")
			case "style-only":
				req.ImageInputs = req.ImageInputs[1:]
			case "source-not-first":
				req.ImageInputs[0], req.ImageInputs[1] = req.ImageInputs[1], req.ImageInputs[0]
			}
			if _, err := FreezeIdeogram("test", p, req); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}
