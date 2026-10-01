package cli

import (
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdeogramRuntimeDiscovery(t *testing.T) {
	t.Setenv("IDEOGRAM_TEST_KEY", "secret-not-in-discovery")
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	data := `generation:
  default-profile: story
  providers:
    ideogram:
      api-key-env: IDEOGRAM_TEST_KEY
      concurrency: 2
      timeout: 5m
  profiles:
    story:
      renderer: ideogram
      model: ideogram-3
      operation: character
      rendering-speed: quality
      magic-prompt: off
      style-type: auto
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := executeProject(t, "renderers", "--config", path)
	if err != nil {
		t.Fatal(err)
	}
	var caps []app.RendererCapability
	if err := json.Unmarshal([]byte(out), &caps); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "secret-not-in-discovery") {
		t.Fatal("API key leaked")
	}
	for _, c := range caps {
		if c.Name == "ideogram" && c.Available && c.Profile == "story" {
			return
		}
	}
	t.Fatalf("configured Ideogram profile not discoverable: %s", out)
}
func TestIdeogramCLIProfileFlag(t *testing.T) {
	_, err := executeProject(t, "asset", "request", "missing.yaml", "--generation-profile", "story", "--prompt", "castle")
	if err == nil || strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("profile flag not supported: %v", err)
	}
}
func TestImageFlagsCannotSilentlyRequestBuiltin(t *testing.T) {
	for _, flag := range []string{"--character-image", "--style-image"} {
		t.Run(flag, func(t *testing.T) {
			s := app.NewService()
			init, e := s.Init(context.Background(), app.InitRequest{Directory: filepath.Join(t.TempDir(), "book")})
			if e != nil {
				t.Fatal(e)
			}
			view, e := s.Inspect(context.Background(), app.InspectRequest{ProjectFile: init.ProjectFile})
			if e != nil {
				t.Fatal(e)
			}
			_, e = executeProject(t, "asset", "request", init.ProjectFile, "--revision", string(view.Revision), "--key", "image-only", "--page", "page-01", "--panel", "p1", "--layer", "hero", flag, "missing.png")
			if e == nil {
				t.Fatal("image flag silently ignored by builtin renderer")
			}
		})
	}
}
