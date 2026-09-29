package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleRelocationAndManualEditProtection(t *testing.T) {
	dir := t.TempDir()
	s := NewService()
	created, err := s.Init(context.Background(), InitRequest{Directory: filepath.Join(dir, "book")})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "exports")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	req := BuildRequest{ProjectFile: created.ProjectFile, PageID: "page-01", BundleRoot: root, Width: 240, Height: 360}
	first, err := s.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.BuildID) != 64 || first.Bundle == "" {
		t.Fatalf("missing bundle identity: %+v", first)
	}
	if err := filepath.WalkDir(first.Bundle, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".ttf" {
			_, err = os.Stat(path + ".LICENSE")
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("font companion license was not preserved: %v", err)
	}
	svg, err := os.ReadFile(filepath.Join(first.Bundle, "page.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), "<g ") || !strings.Contains(string(svg), "assets/") {
		t.Fatal("missing editable groups or assets")
	}
	if !strings.Contains(string(svg), "<text ") || !strings.Contains(string(svg), "@font-face") || !strings.Contains(string(svg), "The moon is closer tonight.") {
		t.Fatal("lettering is not editable native text with a portable font")
	}
	manifest, err := os.ReadFile(filepath.Join(first.Bundle, "composition.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(manifest, &m); err != nil {
		t.Fatal(err)
	}
	if m["build_id"] != first.BuildID || m["scene"] == nil || m["provenance"] == nil {
		t.Fatal("missing composition/provenance")
	}
	before, err := os.ReadFile(first.Output)
	if err != nil {
		t.Fatal(err)
	}
	manual := []byte("manual edit")
	if err = os.WriteFile(filepath.Join(first.Bundle, "page.svg"), manual, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := s.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.BuildID != first.BuildID || second.Bundle == first.Bundle {
		t.Fatal("bundle address or distinct publication incorrect")
	}
	caption := filepath.Join(dir, "book", "assets", "caption.svg")
	original, err := os.ReadFile(caption)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(caption, []byte(strings.ReplaceAll(string(original), "#fff5db", "#ff00ff")), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Build(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if changed.BuildID == first.BuildID {
		t.Fatal("asset edit did not change build identity")
	}
	if err = os.WriteFile(caption, original, 0600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(first.Bundle, "page.svg"))
	if string(got) != string(manual) {
		t.Fatal("hand edits overwritten")
	}
	moved := filepath.Join(dir, "relocated")
	if err = os.Rename(second.Bundle, moved); err != nil {
		t.Fatal(err)
	}
	// Hide original assets: no reference may silently fall back to the source project.
	if err = os.Rename(filepath.Join(dir, "book"), filepath.Join(dir, "originals-hidden")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "relocated.png")
	if _, err = s.Build(context.Background(), BuildRequest{ProjectFile: filepath.Join(moved, "page.yaml"), Output: out, Width: 240, Height: 360}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("relocation changed rendered PNG")
	}
}
