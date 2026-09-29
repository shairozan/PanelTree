package app

import (
	"context"
	"golang.org/x/image/font/gofont/goregular"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildInitializedDemoAndPreserveInputs(t *testing.T) {
	ctx := context.Background()
	s := NewService()
	dir := filepath.Join(t.TempDir(), "book")
	init, err := s.Init(ctx, InitRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "page.png")
	req := BuildRequest{ProjectFile: init.ProjectFile, PageID: "page-01", Output: output, Width: 240, Height: 360}
	got, err := s.Build(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.PageID != "page-01" || got.Width != 240 || got.Height != 360 {
		t.Fatalf("bad result: %+v", got)
	}
	f, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 240 || im.Bounds().Dy() != 360 {
		t.Fatal("wrong export size")
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Build(ctx, req); err == nil {
		t.Fatal("overwrote existing output")
	}
	after, _ := os.ReadFile(output)
	if string(before) != string(after) {
		t.Fatal("output changed")
	}
	req.Output = filepath.Join(dir, "unknown.png")
	req.PageID = "unknown"
	if _, err = s.Build(ctx, req); err == nil {
		t.Fatal("unknown page accepted")
	}
	if _, err = os.Stat(req.Output); !os.IsNotExist(err) {
		t.Fatal("failed build left output")
	}
	req.PageID = ""
	if _, err = s.Build(ctx, req); err == nil {
		t.Fatal("ambiguous page accepted")
	}
	req.PageID = "page-01"
	req.Output = filepath.Join(dir, "assets", "hero.png")
	before, err = os.ReadFile(req.Output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Build(ctx, req); err == nil {
		t.Fatal("source overwrite accepted")
	}
	after, _ = os.ReadFile(req.Output)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
}

func TestBuildSVGAndLettering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "font.ttf"), goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prop.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#ffffff"/></svg>`), 0600); err != nil {
		t.Fatal(err)
	}
	source := `schema: paneltree/v0.1
page:
  id: lettered
  canvas: {width: 200, height: 100}
  layout: {panel: p}
  panels:
    - id: p
      layers:
        - id: prop
          source: {kind: svg, path: prop.svg}
        - id: words
          source: {kind: text, text: "A small moon above the hills", font: font.ttf, font_size: 16}
          frame: {x: 0, y: 0, width: 0.5, height: 1}
`
	path := filepath.Join(dir, "page.yaml")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewService()
	if _, err := s.Inspect(context.Background(), InspectRequest{ProjectFile: path}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Build(context.Background(), BuildRequest{ProjectFile: path, Output: filepath.Join(dir, "out.png")}); err != nil {
		t.Fatal(err)
	}
}
