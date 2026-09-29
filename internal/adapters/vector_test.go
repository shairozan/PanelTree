package adapters

import (
	"context"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"golang.org/x/image/font/gofont/goregular"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func asset(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func req(s model.Source, dir string, w, h float64) render.Request {
	return render.Request{Source: s, BaseDir: dir, Scene: scene.Context{Bounds: scene.Rect{Width: w, Height: h}, PixelSize: model.Canvas{Width: w, Height: h}}}
}
func TestBoundedSVGRaster(t *testing.T) {
	dir := t.TempDir()
	asset(t, dir, "prop.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10" viewBox="0 0 20 10"><rect width="20" height="10" fill="#ff0000"/><circle cx="10" cy="5" r="4" fill="#0000ff" opacity="0.5"/></svg>`))
	im, err := (Builtin{}).RasterScene(context.Background(), req(model.Source{Kind: "svg", Path: "prop.svg"}, dir, 40, 20))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 40 || im.Bounds().Dy() != 20 {
		t.Fatal("resolution ignored")
	}
	c := color.NRGBAModel.Convert(im.At(20, 10)).(color.NRGBA)
	if c.R < 126 || c.R > 128 || c.B < 127 || c.B > 129 || c.A != 255 {
		t.Fatalf("wrong alpha blend: %v", c)
	}
}
func TestSVGRejectsUnsupportedFeatures(t *testing.T) {
	dir := t.TempDir()
	for _, body := range []string{`<script/>`, `<image href="https://example.com/a.png"/>`, `<path d="M0 0"/>`, `<rect width="5" height="5" filter="url(#x)"/>`, `<g opacity=".5"><rect width="1" height="1"/></g>`, `<rect width="1" height="1" transform="rotate(30)"/>`} {
		asset(t, dir, "bad.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">`+body+`</svg>`))
		if _, err := (Builtin{}).RasterScene(context.Background(), req(model.Source{Kind: "svg", Path: "bad.svg"}, dir, 10, 10)); err == nil {
			t.Fatalf("accepted unsupported %s", body)
		}
	}
}

func TestSVGPolygonNonzeroWinding(t *testing.T) {
	dir := t.TempDir()
	asset(t, dir, "shape.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><polygon points="0,0 10,0 10,10 0,0 10,0 10,10" fill="#ff0000"/></svg>`))
	im, err := (Builtin{}).RasterScene(context.Background(), req(model.Source{Kind: "svg", Path: "shape.svg"}, dir, 10, 10))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := im.At(8, 2).RGBA()
	if alpha != 65535 {
		t.Fatal("polygon does not follow SVG nonzero winding")
	}
}

func TestSVGContainsWideBannerWithoutOversizedRaster(t *testing.T) {
	dir := t.TempDir()
	asset(t, dir, "banner.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="10"><rect width="1000" height="10"/></svg>`))
	im, err := (Builtin{}).RasterScene(context.Background(), req(model.Source{Kind: "svg", Path: "banner.svg"}, dir, 1000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 1000 || im.Bounds().Dy() != 10 {
		t.Fatalf("contain raster: %v", im.Bounds())
	}
}

func TestSVGRequiresPortableNamespace(t *testing.T) {
	dir := t.TempDir()
	asset(t, dir, "no-namespace.svg", []byte(`<svg width="10" height="10"><rect width="10" height="10"/></svg>`))
	if _, err := (Builtin{}).RasterScene(context.Background(), req(model.Source{Kind: "svg", Path: "no-namespace.svg"}, dir, 10, 10)); err == nil {
		t.Fatal("accepted nonportable SVG without namespace")
	}
}
func TestTextRealMeasurementAndRaster(t *testing.T) {
	dir := t.TempDir()
	asset(t, dir, "font.ttf", goregular.TTF)
	s := model.Source{Kind: "text", Text: "A small moon above the hills", Font: "font.ttf", FontSize: 16, Color: "#ffffff"}
	provider := Builtin{}
	measure := func(w float64) scene.Metrics {
		m, err := provider.Measure(context.Background(), scene.MeasureRequest{Source: s, BaseDir: dir, Constraints: scene.Constraints{Max: model.Canvas{Width: w, Height: 200}}})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	wide, narrow := measure(220), measure(90)
	if narrow.Preferred.Height <= wide.Preferred.Height {
		t.Fatalf("width not used: %+v %+v", wide, narrow)
	}
	im, err := provider.RasterScene(context.Background(), req(s, dir, 90, 200))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for y := 0; y < im.Bounds().Dy(); y++ {
		for x := 0; x < im.Bounds().Dx(); x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a != 0 {
				found = true
				if float64(y) >= narrow.Preferred.Height {
					t.Fatal("pixels exceed measured height")
				}
			}
		}
	}
	if !found {
		t.Fatal("blank text")
	}
	for _, tc := range []struct {
		name string
		s    model.Source
		w, h float64
	}{{"missing font", model.Source{Kind: "text", Text: "Hi", Font: "missing.ttf", FontSize: 16}, 90, 200}, {"overflow", s, 90, 5}, {"word", s, 2, 200}, {"unicode", model.Source{Kind: "text", Text: "moon 🌙", Font: "font.ttf", FontSize: 16}, 90, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := provider.RasterScene(context.Background(), req(tc.s, dir, tc.w, tc.h)); err == nil {
				t.Fatal("invalid text accepted")
			}
		})
	}
	s.Font = "absent.ttf"
	_, err = provider.Measure(context.Background(), scene.MeasureRequest{Source: s, BaseDir: dir, Constraints: scene.Constraints{Max: model.Canvas{Width: 100, Height: 100}}})
	if err == nil || !strings.Contains(err.Error(), "absent.ttf") {
		t.Fatalf("missing font context: %v", err)
	}
}
