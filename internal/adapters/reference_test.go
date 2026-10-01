package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	assets "github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exercise the wire contract so an older adapter cannot silently discard inputs.
func TestRejectUnconsumedReferencePixels(t *testing.T) {
	r := comfyRequest()
	if err := json.Unmarshal([]byte(`{"image_inputs":[{"role":"body/front","sha256":"bad","png":"YWJj"}]}`), &r); err != nil {
		t.Fatal(err)
	}
	if _, err := (&ComfyUI{Profile: comfyProfile()}).Freeze(r); err == nil {
		t.Fatal("reference pixels silently discarded by a text-only profile")
	}
}

func TestImageBindingMustReachOutput(t *testing.T) {
	p := comfyProfile()
	p.ImageBindings = []ImageBinding{{Role: "reference", Node: "image"}}
	p.Workflow["image"] = ComfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": ""}}
	if e := p.validate(); e == nil {
		t.Fatal("disconnected image would falsely claim conditioning")
	}
	p.Workflow["out"].Inputs["mask"] = []any{"image", 1}
	if e := p.validate(); e == nil {
		t.Fatal("mask-only path falsely claims RGB conditioning")
	}
}

func TestFrozenImageUpload(t *testing.T) {
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 64))); e != nil {
		t.Fatal(e)
	}
	pixels := b.Bytes()
	id := assets.Digest(pixels)
	uploads, posts := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/object_info":
			_, _ = w.Write([]byte(`{"Fake":{},"SaveImage":{},"LoadImage":{}}`))
		case "/upload/image":
			uploads++
			f, h, e := r.FormFile("image")
			if e != nil {
				t.Error(e)
				http.Error(w, "bad upload", 400)
				return
			}
			data, e := io.ReadAll(f)
			_ = f.Close()
			if e != nil || !bytes.Equal(data, pixels) || h.Filename != id+".png" || r.FormValue("subfolder") != "paneltree" {
				t.Error("reference bytes or identity changed")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"name": h.Filename, "subfolder": "paneltree", "type": "input"})
		case "/prompt":
			posts++
			if uploads != 1 {
				t.Error("submitted before reference upload")
			}
			var v struct {
				Prompt map[string]ComfyNode `json:"prompt"`
			}
			if e := json.NewDecoder(r.Body).Decode(&v); e != nil {
				t.Error(e)
			}
			if v.Prompt["image"].Inputs["image"] != "paneltree/"+id+".png" {
				t.Error("pixels not connected to workflow")
			}
			_, _ = w.Write([]byte(`{"prompt_id":"id"}`))
		case "/history/id":
			_, _ = w.Write([]byte(`{"id":{"status":{"completed":true,"status_str":"success"},"outputs":{"out":{"images":[{"filename":"a.png","type":"output"}]}}}}`))
		case "/view":
			_, _ = w.Write(pixels)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := comfyProfile()
	p.ImageBindings = []ImageBinding{{Role: "reference", Node: "image"}}
	p.Workflow["image"] = ComfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": ""}}
	p.Workflow["out"].Inputs["images"] = []any{"image", 0}
	c := &ComfyUI{URL: srv.URL, Profile: p}
	r := comfyRequest()
	r.ImageInputs = []render.ImageInput{{Role: "reference", SHA256: id, PNG: bytes.Clone(pixels)}}
	recipe, e := c.Freeze(r)
	if e != nil {
		t.Fatal(e)
	}
	r.ImageInputs[0].PNG[0] = 0
	if _, e = c.Generate(context.Background(), recipe); e != nil {
		t.Fatal(e)
	}
	if posts != 1 {
		t.Fatal("missing submission")
	}
}

func TestReferenceCapabilityFailures(t *testing.T) {
	var b bytes.Buffer
	if e := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 64))); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"count", "role", "hash", "format"} {
		t.Run(mode, func(t *testing.T) {
			p := comfyProfile()
			p.ImageBindings = []ImageBinding{{Role: "reference", Node: "image"}}
			p.Workflow["image"] = ComfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": ""}}
			p.Workflow["out"].Inputs["images"] = []any{"image", 0}
			r := comfyRequest()
			r.ImageInputs = []render.ImageInput{{Role: "reference", SHA256: assets.Digest(b.Bytes()), PNG: b.Bytes()}}
			switch mode {
			case "count":
				r.ImageInputs = append(r.ImageInputs, r.ImageInputs[0])
			case "role":
				r.ImageInputs[0].Role = "unrequested"
			case "hash":
				r.ImageInputs[0].SHA256 = "bad"
			case "format":
				r.ImageInputs[0].PNG = []byte("not PNG")
				r.ImageInputs[0].SHA256 = assets.Digest(r.ImageInputs[0].PNG)
			}
			if _, e := (&ComfyUI{Profile: p}).Freeze(r); e == nil {
				t.Fatal("unsupported reference input accepted")
			}
		})
	}
}
