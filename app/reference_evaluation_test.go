package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in; ordinary CI never contacts a GPU. The fixture is authored geometric
// artwork, explicitly accepted only for this evaluation, never a user's design.
func TestRealReferenceEvaluation(t *testing.T) {
	endpoint := os.Getenv("PANELTREE_REFERENCE_EVAL_URL")
	if endpoint == "" {
		t.Skip("opt-in real ComfyUI evaluation")
	}
	output := os.Getenv("PANELTREE_REFERENCE_EVAL_OUTPUT")
	if output == "" {
		t.Fatal("set evaluation output directory")
	}
	if e := os.MkdirAll(output, 0700); e != nil {
		t.Fatal(e)
	}
	textProfile, e := adapters.LoadComfyProfile("../examples/comfyui/sd15-profile.json")
	if e != nil {
		t.Fatal(e)
	}
	imageProfile, e := adapters.LoadComfyProfile("../examples/comfyui/sd15-reference-profile.json")
	if e != nil {
		t.Fatal(e)
	}
	// Use the exact local backend identities of the verified image profile.
	textProfile.BackendIdentity = imageProfile.BackendIdentity
	textProfile.Models = imageProfile.Models
	textProfile.Workflow["4"] = imageProfile.Workflow["4"]
	_, p, i := characterFixture(t)
	ctx := context.Background()
	c := &adapters.ComfyUI{URL: endpoint, Profile: imageProfile}
	s := NewService(WithComfyUI(c))
	r := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "create", License: "CC0-1.0", Attribution: "PanelTree synthetic geometric fixture"}
	set, e := s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	for _, slot := range referenceSlots {
		name := strings.ReplaceAll(slot, "/", "-") + ".png"
		data := referenceFigure(t, slot)
		for _, path := range []string{filepath.Join(filepath.Dir(p), "assets", name), filepath.Join(output, name)} {
			if e = os.WriteFile(path, data, 0600); e != nil {
				t.Fatal(e)
			}
		}
		r.Action = "import"
		r.Slot = slot
		r.Path = "assets/" + name
		r.Revision = set.Revision
		set, e = s.Reference(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
		for id, candidate := range set.Candidates {
			if candidate.Slot == slot {
				r.Candidate = id
			}
		}
		r.Action = "accept"
		r.Revision = set.Revision
		set, e = s.Reference(ctx, r)
		if e != nil {
			t.Fatal(e)
		}
	}
	r.Action = "publish"
	r.Version = "1"
	r.Revision = set.Revision
	set, e = s.Reference(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	save := func(name string, data []byte) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(output, name), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	manifest, _ := json.MarshalIndent(set, "", "  ")
	save("approved-reference-set.json", manifest)
	// Exercise every derived slot against frozen accepted parents. Outputs remain
	// unapproved candidates: the test does not assert perceptual correctness.
	for n, slot := range referenceSlots[1:] {
		req := ReferenceRequest{ProjectFile: p, Set: "hero", Action: "request", Revision: set.Revision, Slot: slot, Key: "derived-" + strings.ReplaceAll(slot, "/", "-"), Width: 512, Height: 512, Generation: &render.Generation{Prompt: "flat comic illustration, short black hair, blue coat, red patch on anatomical left sleeve", Seed: uint64(17 + n)}, License: "CreativeML Open RAIL-M", Attribution: "SD 1.5; PanelTree CC0 reference fixture"}
		set, e = s.Reference(ctx, req)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
			t.Fatal(e)
		}
		store, e := s.openJobs(ctx, p)
		if e != nil {
			t.Fatal(e)
		}
		_, payload, pixels, e := store.Result(ctx, set.Jobs[req.Key])
		if e != nil {
			t.Fatal(e)
		}
		save(req.Key+".png", pixels)
		save(req.Key+".json", payload)
	}
	var samples []image.Image
	writeCharacter(t, p, strings.ReplaceAll(strings.ReplaceAll(characterFixtureJSON, "orange coat", "blue coat"), "#ef6848", "#194bb4"))
	doc := i.Documents[2].Document
	hero := doc.Page.Panels[0].Layers[1].Children[0]
	hero.Frame = nil
	hero.Source.Character.Props = nil
	hero.Source.Character.Pose = ""
	hero.Source.Character.Expression = ""
	doc.Page.Canvas = model.Canvas{Width: 512, Height: 512}
	doc.Page.Layout = model.Layout{Panel: "p1"}
	doc.Page.Panels = []model.Panel{{ID: "p1", Layers: []model.Layer{hero}}}
	if _, e = s.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: i.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: doc}}}); e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"text", "image"} {
		profile := textProfile
		if mode == "image" {
			profile = imageProfile
		}
		service := NewService(WithComfyUI(&adapters.ComfyUI{URL: endpoint, Profile: profile}))
		view, e := service.Inspect(ctx, InspectRequest{ProjectFile: p})
		if e != nil {
			t.Fatal(e)
		}
		doc := view.Documents[2].Document
		use := doc.Page.Panels[0].Layers[0].Source.Character
		use.ReferenceSet = nil
		if mode == "image" {
			use.ReferenceSet = &model.ReferenceSelection{Set: "hero", Version: "1", Directions: []string{"front", "left", "right", "rear"}, Packing: "cards-row/v1"}
		}
		edited, e := service.Edit(ctx, EditRequest{ProjectFile: p, ExpectedRevision: view.Revision, Edits: []DocumentEdit{{File: "pages/01.yaml", Document: doc}}})
		if e != nil {
			t.Fatal(e)
		}
		for n, prompt := range []string{"full body comic illustration, short black hair, blue coat, red patch on left sleeve, smiling and waving in a park", "comic portrait, short black hair, blue coat, red patch on left sleeve, surprised expression, looking to the left"} {
			for _, seed := range []uint64{17, 42} {
				name := mode + "-" + []string{"wave", "portrait"}[n] + "-" + map[uint64]string{17: "17", 42: "42"}[seed]
				req := characterRequest(p, edited.Revision, name, target())
				req.Generation = &render.Generation{Prompt: prompt, Seed: seed}
				req.Width = 512
				req.Height = 512
				j, e := service.RequestAsset(ctx, req)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = service.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
					t.Fatal(e)
				}
				store, e := service.openJobs(ctx, p)
				if e != nil {
					t.Fatal(e)
				}
				_, payload, pixels, e := store.Result(ctx, j.ID)
				if e != nil {
					status, _ := store.Get(ctx, j.ID)
					t.Fatalf("%v: %+v", e, status)
				}
				save(name+".png", pixels)
				save(name+".json", payload)
				im, e := png.Decode(bytes.NewReader(pixels))
				if e != nil {
					t.Fatal(e)
				}
				samples = append(samples, im)
			}
		}
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 256*4, 256*2))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for n, im := range samples {
		b := im.Bounds()
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				sheet.Set((n%4)*256+x, (n/4)*256+y, im.At(b.Min.X+x*b.Dx()/256, b.Min.Y+y*b.Dy()/256))
			}
		}
	}
	var encoded bytes.Buffer
	if e = png.Encode(&encoded, sheet); e != nil {
		t.Fatal(e)
	}
	save("comparison.png", encoded.Bytes())
}

func referenceFigure(t *testing.T, slot string) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 512, 512))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	rect := func(x, y, w, h int, c color.RGBA) {
		draw.Draw(im, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Src)
	}
	skin := color.RGBA{220, 166, 126, 255}
	blue := color.RGBA{25, 75, 180, 255}
	black := color.RGBA{25, 25, 30, 255}
	red := color.RGBA{210, 35, 45, 255}
	head := strings.HasPrefix(slot, "head/")
	rear := strings.HasSuffix(slot, "/rear")
	left := strings.HasSuffix(slot, "/left")
	right := strings.HasSuffix(slot, "/right")
	if head && (left || right) {
		x := 184
		rect(x, 80, 144, 288, skin)
		rect(x, 64, 144, 80, black)
		rect(x, 370, 144, 95, blue)
		if left {
			rect(x-16, 195, 20, 45, skin)
			rect(x+20, 175, 14, 18, black)
			rect(x+30, 225, 12, 25, red)
		} else {
			rect(x+140, 195, 20, 45, skin)
			rect(x+110, 175, 14, 18, black)
		}
	} else if head {
		rect(144, 80, 224, 288, skin)
		rect(144, 64, 224, 80, black)
		if rear {
			rect(144, 120, 224, 200, black)
		} else {
			rect(197, 175, 14, 18, black)
			rect(300, 175, 14, 18, black)
			rect(323, 221, 12, 25, red)
		}
		rect(130, 370, 252, 95, blue)
	} else if left || right {
		// Narrow torso and one visible arm/leg show an actual side silhouette;
		// the far limbs are occluded, not two frontal limbs with a turned face.
		rect(232, 40, 48, 90, skin)
		rect(232, 28, 48, 30, black)
		rect(224, 136, 64, 188, blue)
		rect(238, 324, 36, 140, black)
		rect(239, 145, 30, 165, blue)
		rect(239, 310, 30, 30, skin)
		if left {
			rect(220, 76, 15, 20, skin)
			rect(239, 66, 7, 9, black)
			rect(242, 88, 4, 8, red)
			rect(244, 177, 20, 30, red)
		} else {
			rect(277, 76, 15, 20, skin)
			rect(266, 66, 7, 9, black)
		}
	} else {
		rect(220, 40, 72, 90, skin)
		rect(220, 28, 72, 30, black)
		rect(193, 136, 126, 188, blue)
		rect(160, 145, 30, 165, blue)
		rect(322, 145, 30, 165, blue)
		rect(160, 310, 30, 30, skin)
		rect(322, 310, 30, 30, skin)
		rect(197, 324, 48, 140, black)
		rect(266, 324, 48, 140, black)
		if rear {
			rect(220, 58, 72, 55, black)
			rect(165, 177, 20, 30, red)
		} else {
			rect(236, 66, 7, 9, black)
			rect(270, 66, 7, 9, black)
			rect(327, 177, 20, 30, red)
			rect(278, 88, 4, 8, red)
		}
	}
	var out bytes.Buffer
	if e := png.Encode(&out, im); e != nil {
		t.Fatal(e)
	}
	return out.Bytes()
}
