package app

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/render"
	"os"
	"path/filepath"
	"testing"
)

func TestArtworkImportAndMedia(t *testing.T) {
	s, p, view := editFixture(t)
	ctx := context.Background()
	original, e := os.ReadFile(filepath.Join(filepath.Dir(p), "assets/hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	uploaded, e := s.ImportArtwork(ctx, ArtworkRequest{ProjectFile: p, PNG: original, License: "artist-owned", Attribution: "Patrick fixture"})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.Media(ctx, p, uploaded.Path, "")
	if e != nil || !bytes.Equal(got, original) {
		t.Fatal("original lost", e)
	}
	all, e := s.Artworks(ctx, p)
	if e != nil || len(all) != 1 || all[0].License != "artist-owned" {
		t.Fatal("provenance lost", e)
	}
	after, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	if after.Revision != view.Revision {
		t.Fatal("import selected artwork automatically")
	}
	if _, e = s.Media(ctx, p, "../../secret.png", ""); e == nil {
		t.Fatal("outside project read")
	}
	if _, e = s.ImportArtwork(ctx, ArtworkRequest{ProjectFile: p, PNG: []byte("not an image")}); e == nil {
		t.Fatal("invalid upload accepted")
	}
	jobs, e := s.Jobs(ctx, p)
	if e != nil || len(jobs) != 0 {
		t.Fatal("import queued generation")
	}
}

func TestPostgresArtworkProvenance(t *testing.T) {
	s, p := postgresFixture(t)
	ctx := context.Background()
	data, e := s.Media(ctx, p, "assets/hero.png", "")
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.ImportArtwork(ctx, ArtworkRequest{ProjectFile: p, PNG: data, License: "artist-owned", Attribution: "Original artist"})
	if e != nil {
		t.Fatal(e)
	}
	items, e := s.Artworks(ctx, p)
	if e != nil || len(items) != 1 || items[0] != a {
		t.Fatalf("import metadata lost: %+v %v", items, e)
	}
	got, e := s.Media(ctx, p, a.Path, "")
	if e != nil || !bytes.Equal(data, got) {
		t.Fatal("original lost", e)
	}
}

func TestFrozenJobSourceAndCapabilities(t *testing.T) {
	_, p, view := editFixture(t)
	t.Setenv("IDEOGRAM_API_KEY", "test-only")
	cfg := render.GenerationConfig{DefaultProfile: "edit", Profiles: map[string]render.GenerationProfile{"edit": {Renderer: "ideogram", Model: "ideogram-4-5", Operation: "edit", MagicPrompt: "off", Quality: "high", Size: "source"}}}
	s, e := NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.RequestAsset(context.Background(), AssetRequest{ProjectFile: p, ExpectedRevision: view.Revision, Target: target(), Renderer: "ideogram", IdempotencyKey: "frozen", Generation: &render.Generation{Prompt: "point and laugh", CharacterReference: "assets/hero.png"}, Width: 512, Height: 768})
	if e != nil {
		t.Fatal(e)
	}
	original, e := os.ReadFile(filepath.Join(filepath.Dir(p), "assets/hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(filepath.Dir(p), "assets/hero.png"), []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	data, e := s.JobSource(context.Background(), p, j.ID)
	if e != nil || !bytes.Equal(data, original) {
		t.Fatal("frozen source missing", e)
	}
	caps, _ := json.Marshal(s.Renderers())
	if !bytes.Contains(caps, []byte(`"quality":"high"`)) || !bytes.Contains(caps, []byte(`"size":"source"`)) {
		t.Fatalf("profile settings missing: %s", caps)
	}
}
