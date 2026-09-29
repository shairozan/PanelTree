package app

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncrementalBuild(t *testing.T) {
	ctx := context.Background()
	s := NewService()
	dir := filepath.Join(t.TempDir(), "book")
	init, err := s.Init(ctx, InitRequest{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "pages", "01.yaml")
	original, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	build := func() BuildResult {
		count++
		got, e := s.Build(ctx, BuildRequest{ProjectFile: init.ProjectFile, PageID: "page-01", Output: filepath.Join(dir, fmt.Sprintf("out-%d.png", count)), Width: 120, Height: 180})
		if e != nil {
			t.Fatal(e)
		}
		return got
	}
	cold := build()
	uncached, e := s.Build(ctx, BuildRequest{ProjectFile: init.ProjectFile, PageID: "page-01", Output: filepath.Join(dir, "uncached.png"), Width: 120, Height: 180, NoCache: true})
	if e != nil {
		t.Fatal(e)
	}
	assertPixels := func(first, second string) {
		t.Helper()
		a, e := os.ReadFile(first)
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(second)
		if e != nil {
			t.Fatal(e)
		}
		ai, e := png.Decode(bytes.NewReader(a))
		if e != nil {
			t.Fatal(e)
		}
		bi, e := png.Decode(bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		for y := 0; y < 180; y++ {
			for x := 0; x < 120; x++ {
				ar, ag, ab, aa := ai.At(x, y).RGBA()
				br, bg, bb, ba := bi.At(x, y).RGBA()
				if ar != br || ag != bg || ab != bb || aa != ba {
					t.Fatalf("cache changed pixels at %d,%d: %v %v", x, y, ai.At(x, y), bi.At(x, y))
				}
			}
		}
	}
	assertPixels(cold.Output, uncached.Output)
	if cold.LeafRenders == 0 || cold.Recompositions == 0 {
		t.Fatalf("cold build did no reported work: %+v", cold)
	}
	warm := build()
	if warm.LeafRenders != 0 || warm.Recompositions != 0 {
		t.Fatalf("warm build repeated work: %+v", warm)
	}
	a, _ := os.ReadFile(cold.Output)
	b, _ := os.ReadFile(warm.Output)
	if string(a) != string(b) {
		t.Fatal("warm pixels differ")
	}
	write := func(text string) {
		t.Helper()
		if e := os.WriteFile(page, []byte(text), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("# comment only\n" + string(original))
	comments := build()
	if comments.LeafRenders != 0 || comments.Recompositions != 0 || comments.BuildID != cold.BuildID {
		t.Fatal("comment invalidated build")
	}
	moved := strings.Replace(string(original), "x: 0.2, y: 0.1", "x: 0.3, y: 0.1", 1)
	write(moved)
	movement := build()
	if movement.LeafRenders != 0 || movement.Recompositions == 0 || movement.Recompositions >= cold.Recompositions {
		t.Fatalf("movement invalidation: %+v", movement)
	}
	write(strings.Replace(moved, "The moon is closer tonight.", "The moon is bright tonight.", 1))
	dialogue := build()
	if dialogue.LeafRenders != 1 || dialogue.Recompositions == 0 || dialogue.Recompositions >= cold.Recompositions {
		t.Fatalf("dialogue invalidation: %+v", dialogue)
	}
	// Force the export to miss while reusing transparent compositions.
	write(strings.Replace(string(original), "'#ffffff'", "'#eeeeee'", 1))
	changedBG := build()
	u, e := s.Build(ctx, BuildRequest{ProjectFile: init.ProjectFile, PageID: "page-01", Output: filepath.Join(dir, "uncached-bg.png"), Width: 120, Height: 180, NoCache: true})
	if e != nil {
		t.Fatal(e)
	}
	assertPixels(changedBG.Output, u.Output)
	write(strings.ReplaceAll(string(original), "kind: image, path: ../assets/hero.png", "kind: image, path: ../assets/hero.png, draft: {revision: 1, seed: 42}"))
	draft := build()
	if draft.LeafRenders != 1 || draft.Recompositions == 0 {
		t.Fatalf("explicit draft did not invalidate shared production: %+v", draft)
	}
	write(strings.ReplaceAll(string(original), "kind: image, path: ../assets/hero.png", "kind: image, path: ../assets/hero.png, draft: {revision: 1, seed: 43}"))
	seed := build()
	if seed.LeafRenders != 1 {
		t.Fatalf("seed did not invalidate production: %+v", seed)
	}
}

func TestCacheRelocationAndMask(t *testing.T) {
	ctx := context.Background()
	s := NewService()
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	run := func(dir, name string) BuildResult {
		t.Helper()
		got, e := s.Build(ctx, BuildRequest{ProjectFile: filepath.Join(dir, "project.yaml"), PageID: "page-01", Output: filepath.Join(dir, name+".png"), Width: 120, Height: 180, CacheDir: cacheDir})
		if e != nil {
			t.Fatal(e)
		}
		return got
	}
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, dir := range []string{first, second} {
		if _, e := s.Init(ctx, InitRequest{Directory: dir}); e != nil {
			t.Fatal(e)
		}
	}
	cold := run(first, "cold")
	// Rename an authored source as well as relocating the entire project.
	if e := os.Rename(filepath.Join(second, "assets", "hero.png"), filepath.Join(second, "assets", "actor.png")); e != nil {
		t.Fatal(e)
	}
	page := filepath.Join(second, "pages", "01.yaml")
	data, e := os.ReadFile(page)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(page, []byte(strings.ReplaceAll(string(data), "hero.png", "actor.png")), 0600); e != nil {
		t.Fatal(e)
	}
	moved := run(second, "relocated")
	if moved.LeafRenders != 0 || moved.Recompositions != 0 || moved.BuildID != cold.BuildID {
		t.Fatalf("path relocation invalidated graph: %+v", moved)
	}
	// Replace mask content with an existing opaque PNG. Only its branch rebuilds.
	opaque, e := os.ReadFile(filepath.Join(second, "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(second, "assets", "iris.png"), opaque, 0600); e != nil {
		t.Fatal(e)
	}
	mask := run(second, "mask")
	if mask.Recompositions == 0 || mask.Recompositions >= cold.Recompositions || mask.BuildID == cold.BuildID {
		t.Fatalf("mask dependency missing: %+v", mask)
	}
}
