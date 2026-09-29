package compose

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/model"
	"image"
	"image/color"
	"testing"
)

type fixture map[string]image.Image

func (f fixture) Raster(_ context.Context, s model.Source, _ string) (image.Image, error) {
	im, ok := f[s.Path]
	if !ok {
		return nil, fmt.Errorf("missing %s", s.Path)
	}
	return im, nil
}
func solid(c color.NRGBA) image.Image {
	im := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	im.SetNRGBA(0, 0, c)
	return im
}
func num(v float64) *float64 { return &v }
func leaf(id, path string) model.Layer {
	return model.Layer{ID: model.ID(id), Source: &model.Source{Kind: "image", Path: path}, Fit: "cover"}
}
func page(layers ...model.Layer) model.Page {
	return model.Page{ID: "page", Canvas: model.Canvas{Width: 4, Height: 4}, Layout: model.Layout{Panel: "panel"}, Panels: []model.Panel{{ID: "panel", Layers: layers}}}
}
func renderPage(t *testing.T, p model.Page, f fixture) *image.RGBA {
	t.Helper()
	r, err := layout.Resolve(context.Background(), p, layout.Options{})
	if err != nil {
		t.Fatal(err)
	}
	im, err := Page(context.Background(), r, "", color.Transparent, f)
	if err != nil {
		t.Fatal(err)
	}
	return im
}
func pixel(t *testing.T, im image.Image, x, y int, w color.NRGBA) {
	t.Helper()
	g := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
	for _, v := range [][2]uint8{{g.R, w.R}, {g.G, w.G}, {g.B, w.B}, {g.A, w.A}} {
		d := int(v[0]) - int(v[1])
		if d < -1 || d > 1 {
			t.Fatalf("pixel %d,%d got%v want%v", x, y, g, w)
		}
	}
}

func TestSourceOverAndIsolatedGroupOpacity(t *testing.T) {
	f := fixture{"red": solid(color.NRGBA{R: 255, A: 255}), "blue": solid(color.NRGBA{B: 255, A: 255})}
	group := model.Layer{ID: "group", Opacity: num(.5), Children: []model.Layer{leaf("red", "red"), leaf("blue", "blue")}}
	im := renderPage(t, page(group), f)
	pixel(t, im, 2, 2, color.NRGBA{B: 255, A: 128})
	top := leaf("top", "blue")
	top.Opacity = num(.5)
	im = renderPage(t, page(leaf("bottom", "red"), top), f)
	pixel(t, im, 2, 2, color.NRGBA{R: 127, B: 128, A: 255})
}
func TestNestedTransformsMaskAndPanelClip(t *testing.T) {
	mask := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	mask.SetNRGBA(0, 0, color.NRGBA{A: 255})
	l := leaf("leaf", "red")
	l.Mask = "mask"
	group := model.Layer{ID: "group", Frame: &model.Frame{X: .5, Y: 0, Width: 1, Height: 1}, Children: []model.Layer{l}}
	im := renderPage(t, page(group), fixture{"red": solid(color.NRGBA{R: 255, A: 255}), "mask": mask})
	pixel(t, im, 0, 0, color.NRGBA{})
	pixel(t, im, 2, 0, color.NRGBA{R: 255, A: 255})
	// Rotate a two-color source, then clip to the panel.
	stripe := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	stripe.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	stripe.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	l = leaf("rotate", "stripe")
	l.Transform = &model.Transform{Rotation: 90}
	im = renderPage(t, page(l), fixture{"stripe": stripe})
	pixel(t, im, 2, 0, color.NRGBA{R: 255, A: 255})
	pixel(t, im, 2, 3, color.NRGBA{B: 255, A: 255})
	l.Transform = &model.Transform{ScaleX: num(.5), ScaleY: num(.5)}
	im = renderPage(t, page(l), fixture{"stripe": stripe})
	pixel(t, im, 0, 0, color.NRGBA{})
	pixel(t, im, 1, 1, color.NRGBA{R: 255, A: 255})
}
func TestContainAndCover(t *testing.T) {
	stripe := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	stripe.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	stripe.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	l := leaf("l", "stripe")
	l.Fit = "contain"
	im := renderPage(t, page(l), fixture{"stripe": stripe})
	pixel(t, im, 0, 0, color.NRGBA{})
	pixel(t, im, 0, 1, color.NRGBA{R: 255, A: 255})
	l.Fit = "cover"
	im = renderPage(t, page(l), fixture{"stripe": stripe})
	pixel(t, im, 0, 0, color.NRGBA{R: 255, A: 255})
	pixel(t, im, 3, 3, color.NRGBA{B: 255, A: 255})
}
func TestCompositionLimitsAndCancellation(t *testing.T) {
	p := page()
	p.Canvas = model.Canvas{Width: 10000, Height: 10000}
	r, err := layout.Resolve(context.Background(), p, layout.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Page(context.Background(), r, "", color.Transparent, fixture{}); err == nil {
		t.Fatal("unbounded output")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err = layout.Resolve(context.Background(), page(), layout.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Page(ctx, r, "", color.Transparent, fixture{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestSharedPixelBoundaryHasNoTransparentSeam(t *testing.T) {
	p := page()
	p.Canvas = model.Canvas{Width: 10, Height: 10}
	p.Layout = model.Layout{Type: "row", Margin: 2, Children: []model.Layout{{Panel: "a"}, {Panel: "b"}}}
	p.Panels = []model.Panel{{ID: "a", Layers: []model.Layer{leaf("red", "red")}}, {ID: "b", Layers: []model.Layer{leaf("blue", "blue")}}}
	r, err := layout.Resolve(context.Background(), p, layout.Options{Width: 61, Height: 61})
	if err != nil {
		t.Fatal(err)
	}
	im, err := Page(context.Background(), r, "", color.Transparent, fixture{"red": solid(color.NRGBA{R: 255, A: 255}), "blue": solid(color.NRGBA{B: 255, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	pixel(t, im, 30, 20, color.NRGBA{R: 255, A: 255})
	pixel(t, im, 31, 20, color.NRGBA{B: 255, A: 255})
}

func TestGroupMaskAndGutterClipping(t *testing.T) {
	p := page()
	p.Canvas = model.Canvas{Width: 10, Height: 4}
	p.Layout = model.Layout{Type: "row", Gutter: 2, Children: []model.Layout{{Panel: "a"}, {Panel: "b"}}}
	l := leaf("red", "red")
	l.Frame = &model.Frame{X: -1, Y: 0, Width: 3, Height: 1}
	p.Panels = []model.Panel{{ID: "a", Layers: []model.Layer{{ID: "group", Children: []model.Layer{l}, Mask: "mask"}}}, {ID: "b"}}
	im := renderPage(t, p, fixture{"red": solid(color.NRGBA{R: 255, A: 128}), "mask": solid(color.NRGBA{A: 128})})
	pixel(t, im, 1, 1, color.NRGBA{R: 255, A: 64})
	pixel(t, im, 4, 1, color.NRGBA{})
	pixel(t, im, 6, 1, color.NRGBA{})
}

func TestFractionalLayerContentAndMaskEdges(t *testing.T) {
	f := fixture{"red": solid(color.NRGBA{R: 255, A: 255}), "mask": solid(color.NRGBA{A: 255})}
	l := leaf("narrow", "red")
	l.Frame = &model.Frame{X: .375, Y: 0, Width: .25, Height: 1}
	im := renderPage(t, page(l), f)
	pixel(t, im, 1, 1, color.NRGBA{})
	pixel(t, im, 2, 1, color.NRGBA{R: 255, A: 255})
	// A 1:4 source contained in a 4x4 frame occupies the same [1.5,2.5] interval.
	tall := image.NewNRGBA(image.Rect(0, 0, 1, 4))
	for y := 0; y < 4; y++ {
		tall.SetNRGBA(0, y, color.NRGBA{R: 255, A: 255})
	}
	f["tall"] = tall
	l = leaf("contain", "tall")
	l.Fit = "contain"
	im = renderPage(t, page(l), f)
	pixel(t, im, 1, 1, color.NRGBA{})
	pixel(t, im, 2, 1, color.NRGBA{R: 255, A: 255})
	// Children can overflow a group, but a mask crops to its rounded frame.
	child := leaf("child", "red")
	child.Frame = &model.Frame{X: -10, Y: 0, Width: 20, Height: 1}
	group := model.Layer{ID: "masked", Frame: &model.Frame{X: .375, Y: 0, Width: .25, Height: 1}, Mask: "mask", Children: []model.Layer{child}}
	im = renderPage(t, page(group), f)
	pixel(t, im, 1, 1, color.NRGBA{})
	pixel(t, im, 2, 1, color.NRGBA{R: 255, A: 255})
}
