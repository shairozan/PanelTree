package layout

import (
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"math"
	"testing"
)

func FuzzLayoutGeometry(f *testing.F) {
	f.Add(100.0, 200.0, 1.0, 2.0, 4.0, 8.0)
	f.Add(0.0, -1.0, 0.0, math.Inf(1), 0.0, 0.0)
	f.Fuzz(func(t *testing.T, width, height, one, two, margin, gutter float64) {
		page := model.Page{ID: "page", Canvas: model.Canvas{Width: width, Height: height}, Layout: model.Layout{Type: "row", Margin: margin, Gutter: gutter, Children: []model.Layout{{Panel: "a", Size: &model.Size{Weight: &one}}, {Panel: "b", Size: &model.Size{Weight: &two}}}}, Panels: []model.Panel{{ID: "a"}, {ID: "b"}}}
		r, e := Resolve(context.Background(), page, Options{Width: 120, Height: 180, Fit: "contain"})
		if e != nil {
			return
		}
		var walk func(scene.ResolvedNode)
		walk = func(n scene.ResolvedNode) {
			for _, v := range []float64{n.Bounds.X, n.Bounds.Y, n.Bounds.Width, n.Bounds.Height} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatal("successful layout contains non-finite bounds")
				}
			}
			if n.Bounds.Width <= 0 || n.Bounds.Height <= 0 {
				t.Fatal("successful layout has empty bounds")
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(r.Tree())
		again, e := Resolve(context.Background(), page, Options{Width: 120, Height: 180, Fit: "contain"})
		if e != nil {
			t.Fatal(e)
		}
		a, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		b, e := json.Marshal(again)
		if e != nil {
			t.Fatal(e)
		}
		if string(a) != string(b) {
			t.Fatal("layout is nondeterministic")
		}
	})
}
