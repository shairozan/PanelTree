package adapters

import (
	"context"
	"github.com/shairozan/PanelTree/model"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPNGReadAndErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	got, err := (PNG{}).Raster(context.Background(), model.Source{Kind: "image", Path: "asset.png"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(got.At(0, 0)) != (color.NRGBA{R: 255, A: 128}) {
		t.Fatal("alpha lost")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
	for _, name := range []string{"missing.png", "bad.png"} {
		if name == "bad.png" {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("broken"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		_, err := (PNG{}).Raster(context.Background(), model.Source{Kind: "image", Path: name}, dir)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("missing path context: %v", err)
		}
	}
}

func TestPNGDimensionLimit(t *testing.T) {
	// Highly compressible input still must be rejected before pixel decoding.
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "large.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewGray(image.Rect(0, 0, 4097, 1024))); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = (PNG{}).Raster(context.Background(), model.Source{Kind: "image", Path: "large.png"}, dir); err == nil {
		t.Fatal("oversized image accepted")
	}
}
