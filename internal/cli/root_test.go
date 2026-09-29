package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndVersionDoNotLoadConfig(t *testing.T) {
	for _, flag := range []string{"--help", "--version"} {
		t.Run(flag, func(t *testing.T) {
			cmd := Command()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), flag})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ToLower(out.String()), "paneltree") {
				t.Fatalf("unexpected output: %s", out.String())
			}
		})
	}
}

func TestCommandFactoriesAreIsolated(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "")
	first := Command()
	first.SetArgs([]string{"--log-level", "invalid"})
	if err := first.Execute(); err == nil {
		t.Fatal("expected invalid log level")
	}
	second := Command()
	second.SetOut(&bytes.Buffer{})
	second.SetArgs([]string{})
	if err := second.Execute(); err != nil {
		t.Fatalf("fresh command inherited state: %v", err)
	}
}

func TestExplicitMissingConfigFails(t *testing.T) {
	cmd := Command()
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing configuration error")
	}
}
