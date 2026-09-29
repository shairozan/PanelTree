package layout

import (
	"context"
	"errors"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func number(v float64) *float64 { return &v }
func demo(t *testing.T) model.Page {
	t.Helper()
	s, e := project.Load("../project/template/pages/01.yaml")
	if e != nil {
		t.Fatal(e)
	}
	return s.Pages[0].Page
}
func simple(width, height float64, children ...model.Layout) model.Page {
	p := model.Page{ID: "page", Canvas: model.Canvas{Width: width, Height: height}, Layout: model.Layout{Type: "row", Children: children}}
	for _, c := range children {
		p.Panels = append(p.Panels, model.Panel{ID: c.Panel})
	}
	return p
}
func find(t *testing.T, n scene.ResolvedNode, id string) scene.ResolvedNode {
	t.Helper()
	var walk func(scene.ResolvedNode) (scene.ResolvedNode, bool)
	walk = func(n scene.ResolvedNode) (scene.ResolvedNode, bool) {
		if n.ID == id {
			return n, true
		}
		for _, c := range n.Children {
			if r, ok := walk(c); ok {
				return r, true
			}
		}
		return scene.ResolvedNode{}, false
	}
	r, ok := walk(n)
	if !ok {
		t.Fatalf("missing node %s", id)
	}
	return r
}
func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-8 {
		t.Fatalf("got %.12g want %.12g", got, want)
	}
}

func TestCanonicalFivePanelGeometry(t *testing.T) {
	r, e := Resolve(context.Background(), demo(t), Options{})
	if e != nil {
		t.Fatal(e)
	}
	expected := map[string]scene.Rect{"p1": {X: 40, Y: 40, Width: 657.6, Height: 848}, "p2": {X: 721.6, Y: 40, Width: 438.4, Height: 848}, "p3": {X: 40, Y: 912, Width: 1072.0 / 3, Height: 848}, "p4": {X: 40 + 1072.0/3 + 24, Y: 912, Width: 1072.0 / 3, Height: 848}, "p5": {X: 40 + 2*(1072.0/3+24), Y: 912, Width: 1072.0 / 3, Height: 848}}
	for id, w := range expected {
		g := find(t, r.Tree(), id).LogicalBounds
		near(t, g.X, w.X)
		near(t, g.Y, w.Y)
		near(t, g.Width, w.Width)
		near(t, g.Height, w.Height)
	}
}
func TestMixedTracksAndPixelEdges(t *testing.T) {
	p := simple(101, 30, model.Layout{Panel: "a", Size: &model.Size{Percent: number(20)}}, model.Layout{Panel: "b", Size: &model.Size{Weight: number(1)}}, model.Layout{Panel: "c", Size: &model.Size{Weight: number(3)}})
	r, e := Resolve(context.Background(), p, Options{Width: 202, Height: 60})
	if e != nil {
		t.Fatal(e)
	}
	a, b, c := find(t, r.Tree(), "a"), find(t, r.Tree(), "b"), find(t, r.Tree(), "c")
	near(t, a.Bounds.Width, 20.2)
	near(t, b.Bounds.Width, 20.2)
	near(t, c.Bounds.Width, 60.6)
	if a.Pixels.Right != b.Pixels.Left || b.Pixels.Right != c.Pixels.Left || c.Pixels.Right != 202 {
		t.Fatalf("noncontiguous edges: %+v %+v %+v", a.Pixels, b.Pixels, c.Pixels)
	}
}

func TestSharedHalfPixelEdge(t *testing.T) {
	p := simple(10, 10, model.Layout{Panel: "a"}, model.Layout{Panel: "b"})
	p.Layout.Margin = 2
	r, err := Resolve(context.Background(), p, Options{Width: 61, Height: 61})
	if err != nil {
		t.Fatal(err)
	}
	a, b := find(t, r.Tree(), "a"), find(t, r.Tree(), "b")
	if a.Pixels.Right != 31 || b.Pixels.Left != 31 {
		t.Fatalf("shared half-pixel boundary split: %+v %+v", a.Pixels, b.Pixels)
	}
}
func TestInvalidAllocation(t *testing.T) {
	cases := []struct {
		name string
		p    model.Page
	}{
		{"percent over", simple(100, 100, model.Layout{Panel: "a", Size: &model.Size{Percent: number(60)}}, model.Layout{Panel: "b", Size: &model.Size{Percent: number(60)}})},
		{"percent remainder", simple(100, 100, model.Layout{Panel: "a", Size: &model.Size{Percent: number(70)}})},
		{"no weight space", simple(100, 100, model.Layout{Panel: "a", Size: &model.Size{Percent: number(100)}}, model.Layout{Panel: "b"})},
		{"invalid weight", simple(100, 100, model.Layout{Panel: "a", Size: &model.Size{Weight: number(-1)}})},
	}
	margin := simple(100, 100, model.Layout{Panel: "a"})
	margin.Layout.Margin = 51
	cases = append(cases, struct {
		name string
		p    model.Page
	}{"margin", margin})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := Resolve(context.Background(), tc.p, Options{}); e == nil {
				t.Fatal("invalid geometry accepted")
			}
		})
	}
}
func TestOutputFit(t *testing.T) {
	p := simple(100, 200, model.Layout{Panel: "a"})
	if _, e := Resolve(context.Background(), p, Options{Width: 200, Height: 200}); e == nil {
		t.Fatal("aspect mismatch accepted")
	}
	for _, tc := range []struct {
		fit  string
		want scene.PixelRect
	}{{"contain", scene.PixelRect{Left: 50, Top: 0, Right: 150, Bottom: 200}}, {"cover", scene.PixelRect{Left: 0, Top: -100, Right: 200, Bottom: 300}}} {
		r, e := Resolve(context.Background(), p, Options{Width: 200, Height: 200, Fit: tc.fit})
		if e != nil {
			t.Fatal(e)
		}
		if g := find(t, r.Tree(), "a").Pixels; g != tc.want {
			t.Fatalf("%s: got %+v want %+v", tc.fit, g, tc.want)
		}
	}
	a, e := Resolve(context.Background(), p, Options{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := Resolve(context.Background(), p, Options{Width: 200, Height: 400})
	if e != nil {
		t.Fatal(e)
	}
	if find(t, a.Tree(), "a").LogicalBounds != find(t, b.Tree(), "a").LogicalBounds {
		t.Fatal("output size changed logical layout")
	}
}

func TestNativeFractionalCanvas(t *testing.T) {
	p := simple(100.4, 200.2, model.Layout{Panel: "a"})
	r, err := Resolve(context.Background(), p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Output().World != scene.Identity() || r.Output().Width != 100 || r.Output().Height != 200 {
		t.Fatalf("native output must round edges without scaling: %+v", r.Output())
	}
	if find(t, r.Tree(), "a").Pixels != (scene.PixelRect{Right: 100, Bottom: 200}) {
		t.Fatal("native panel edges differ from output")
	}
}

type measureFunc func(context.Context, MeasureRequest) (scene.Metrics, error)

func (f measureFunc) Measure(ctx context.Context, r MeasureRequest) (scene.Metrics, error) {
	return f(ctx, r)
}
func textPage() model.Page {
	p := simple(200, 100, model.Layout{Panel: "a"})
	p.Panels[0].Layers = []model.Layer{{ID: "words", Source: &model.Source{Kind: "text", Text: "wrap me", Font: "font.ttf", FontSize: 12}, Frame: &model.Frame{X: 0, Y: 0, Width: 0.5, Height: 1}}}
	return p
}
func TestMeasurementUsesAssignedWidth(t *testing.T) {
	p := textPage()
	var widths []float64
	m := measureFunc(func(_ context.Context, r MeasureRequest) (scene.Metrics, error) {
		widths = append(widths, r.Constraints.Max.Width)
		if r.BaseDir != filepath.FromSlash("assets") {
			t.Fatal("source context lost")
		}
		return scene.Metrics{Minimum: model.Canvas{Width: 20, Height: 10}, Preferred: model.Canvas{Width: r.Constraints.Max.Width, Height: 4000 / r.Constraints.Max.Width}}, nil
	})
	for _, width := range []float64{200, 100} {
		p.Canvas.Width = width
		if _, e := Resolve(context.Background(), p, Options{BaseDir: filepath.FromSlash("assets"), Measurer: m}); e != nil {
			t.Fatal(e)
		}
	}
	if len(widths) != 2 || widths[0] != 100 || widths[1] != 50 {
		t.Fatalf("measurement widths: %v", widths)
	}
	p.Canvas.Width = 20
	if _, e := Resolve(context.Background(), p, Options{Measurer: m, BaseDir: filepath.FromSlash("assets")}); e == nil {
		t.Fatal("text overflow accepted")
	}
}
func TestMeasurementErrorsAndCancellation(t *testing.T) {
	if _, e := Resolve(context.Background(), textPage(), Options{}); e == nil || !strings.Contains(e.Error(), "measur") {
		t.Fatalf("missing text measurer: %v", e)
	}
	sentinel := errors.New("missing font")
	m := measureFunc(func(context.Context, MeasureRequest) (scene.Metrics, error) { return scene.Metrics{}, sentinel })
	if _, e := Resolve(context.Background(), textPage(), Options{Measurer: m}); !errors.Is(e, sentinel) {
		t.Fatalf("lost error: %v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Resolve(ctx, demo(t), Options{}); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation: %v", e)
	}
}
func TestNestedLayersAndDetachedResult(t *testing.T) {
	p := simple(100, 100, model.Layout{Panel: "a"})
	p.Panels[0].Layers = []model.Layer{{ID: "group", Frame: &model.Frame{X: 0.1, Y: 0.2, Width: 0.8, Height: 0.6}, Children: []model.Layer{{ID: "leaf", Source: &model.Source{Kind: "image", Path: "a.png"}, Frame: &model.Frame{X: 0.25, Y: 0.5, Width: 0.5, Height: 0.5}}}}}
	r, e := Resolve(context.Background(), p, Options{})
	if e != nil {
		t.Fatal(e)
	}
	leaf := find(t, r.Tree(), "leaf")
	if leaf.LogicalBounds != (scene.Rect{X: 30, Y: 50, Width: 40, Height: 30}) {
		t.Fatalf("bounds: %+v", leaf)
	}
	if leaf.Clip != (scene.Rect{Width: 100, Height: 100}) {
		t.Fatalf("clip lost: %+v", leaf.Clip)
	}
	tree := r.Tree()
	tree.Children[0].ID = "changed"
	if r.Tree().Children[0].ID == "changed" {
		t.Fatal("result exposes mutable backing tree")
	}
}

func TestTransformsComposeAroundLayerCenter(t *testing.T) {
	p := simple(100, 100, model.Layout{Panel: "a"})
	p.Panels[0].Layers = []model.Layer{{ID: "turn", Frame: &model.Frame{X: 0.1, Y: 0.2, Width: 0.4, Height: 0.2}, Transform: &model.Transform{ScaleX: number(2), ScaleY: number(1), Rotation: 90}, Source: &model.Source{Kind: "image", Path: "a.png"}}}
	r, e := Resolve(context.Background(), p, Options{})
	if e != nil {
		t.Fatal(e)
	}
	b := find(t, r.Tree(), "turn").LogicalBounds
	near(t, b.X, 20)
	near(t, b.Y, -10)
	near(t, b.Width, 20)
	near(t, b.Height, 80)
}
