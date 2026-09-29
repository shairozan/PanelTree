package build

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/cache"
	"github.com/shairozan/PanelTree/internal/compose"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type versioned struct {
	adapters.Builtin
	textVersion string
}

func (v versioned) CacheRecipe(ctx context.Context, r render.Request) (any, error) {
	recipe, err := v.Builtin.CacheRecipe(ctx, r)
	version := "stable"
	if r.Source.Kind == "text" {
		version = v.textVersion
	}
	return struct {
		Recipe  any
		Version string
	}{recipe, version}, err
}
func testPage(id string) model.Page {
	return model.Page{ID: model.ID(id), Canvas: model.Canvas{Width: 100, Height: 100}, Layout: model.Layout{Panel: "p"}, Panels: []model.Panel{{ID: "p", Layers: []model.Layer{
		{ID: "prop", Source: &model.Source{Kind: "svg", Path: "prop.svg"}},
		{ID: "words", Source: &model.Source{Kind: "text", Text: "Hi", Font: "font.ttf", FontSize: 16}},
	}}}}
}
func TestDependencyGraph(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, b []byte) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(dir, path), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#abcdef"/></svg>`
	write("prop.svg", []byte(svg))
	write("font.ttf", goregular.TTF)
	run := func(page model.Page, version string) *Session {
		t.Helper()
		resolved, e := layout.Resolve(ctx, page, layout.Options{BaseDir: dir, Measurer: adapters.Builtin{}})
		if e != nil {
			t.Fatal(e)
		}
		s, e := New(ctx, store, versioned{textVersion: version}, dir, resolved)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = compose.PageWithMemo(ctx, resolved, dir, nil, s, s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	first, second, unrelated := testPage("first"), testPage("second"), testPage("unrelated")
	unrelated.Panels[0].Layers = unrelated.Panels[0].Layers[1:]
	run(first, "v1")
	run(second, "v1")
	run(unrelated, "v1")
	write("prop.svg", []byte(svg+"\n"))
	a, b, c := run(first, "v1"), run(second, "v1"), run(unrelated, "v1")
	if a.Report.LeafRenders != 1 || b.Report.Recompositions == 0 || c.Report.Recompositions != 0 {
		t.Fatalf("shared source invalidation: %+v %+v %+v", a.Report, b.Report, c.Report)
	}
	write("font.ttf", gobold.TTF)
	font := run(first, "v1")
	if font.Report.LeafRenders != 1 {
		t.Fatalf("font invalidation: %+v", font.Report)
	}
	version := run(first, "v2")
	if version.Report.LeafRenders != 1 {
		t.Fatalf("version invalidation: %+v", version.Report)
	}
	warm := run(first, "v2")
	if warm.Report.LeafRenders != 0 || warm.Report.Recompositions != 0 {
		t.Fatal("warm build did work")
	}
	// Corrupt the root composition blob; valid descendants must repair it.
	root := warm.Report.Events[len(warm.Report.Events)-1]
	if err = os.WriteFile(filepath.Join(store.Root, "blobs", root.ContentHash), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	// Page and panel intentionally share the identical output blob.
	repaired := run(first, "v2")
	if repaired.Report.LeafRenders != 0 || repaired.Report.Recompositions != 2 {
		t.Fatalf("recovery: %+v", repaired.Report)
	}
}

func TestCompositionPreservesPremultipliedPixels(t *testing.T) {
	ctx := context.Background()
	store, e := cache.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := testPage("alpha")
	p.Panels[0].Layers = nil
	resolved, e := layout.Resolve(ctx, p, layout.Options{})
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(ctx, store, adapters.Builtin{}, "", resolved)
	if e != nil {
		t.Fatal(e)
	}
	im := image.NewRGBA(image.Rect(0, 0, 100, 100))
	im.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 7})
	if e = s.Save(ctx, resolved.Tree(), im); e != nil {
		t.Fatal(e)
	}
	got, e := s.Load(ctx, resolved.Tree())
	if e != nil {
		t.Fatal(e)
	}
	if got.RGBAAt(0, 0) != im.RGBAAt(0, 0) {
		t.Fatalf("premultiplied values changed: %v => %v", im.RGBAAt(0, 0), got.RGBAAt(0, 0))
	}
}

func TestCacheDoesNotChangeCompositing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := cache.Open(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10" fill="#1a395b65"/></svg>`)
	if err = os.WriteFile(filepath.Join(dir, "prop.svg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	p := testPage("rounding")
	p.Panels[0].Layers = []model.Layer{{ID: "a", Source: &model.Source{Kind: "svg", Path: "prop.svg"}}, {ID: "b", Source: &model.Source{Kind: "svg", Path: "prop.svg"}}}
	resolved, err := layout.Resolve(ctx, p, layout.Options{})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := compose.Page(ctx, resolved, dir, color.White, adapters.Builtin{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		s, e := New(ctx, store, adapters.Builtin{}, dir, resolved)
		if e != nil {
			t.Fatal(e)
		}
		got, e := compose.PageWithMemo(ctx, resolved, dir, color.White, s, s)
		if e != nil {
			t.Fatal(e)
		}
		if got.RGBAAt(0, 0) != baseline.RGBAAt(0, 0) {
			t.Fatalf("cached grouping changes pixels: %v vs %v", got.RGBAAt(0, 0), baseline.RGBAAt(0, 0))
		}
	}
}
