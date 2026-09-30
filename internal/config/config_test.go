package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestMCPRootsRuntimeConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runtime.yaml")
	if e := os.WriteFile(p, []byte("mcp-roots: ['/books']\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Resolve(viper.New(), p); e != nil {
		t.Fatal(e)
	}
}

func TestInitializerPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, file, env, flag, want string
	}{
		{name: "defaults", want: "info"},
		{name: "file", file: "warn", want: "warn"},
		{name: "environment only", env: "debug", want: "debug"},
		{name: "environment over file", file: "warn", env: "debug", want: "debug"},
		{name: "flag over environment", file: "warn", env: "debug", flag: "error", want: "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PANELTREE_LOG_LEVEL", tc.env)
			var cfg *Config
			v := viper.New()
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("config", "", "")
			cmd.Flags().String("log-level", "info", "")
			if err := v.BindPFlag("log-level", cmd.Flags().Lookup("log-level")); err != nil {
				t.Fatal(err)
			}
			args := []string{}
			if tc.file != "" {
				path := filepath.Join(t.TempDir(), "config.yaml")
				if err := os.WriteFile(path, []byte("log-level: "+tc.file+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--config", path)
			}
			if tc.flag != "" {
				args = append(args, "--log-level", tc.flag)
			}
			cmd.PreRunE = NewInitializer(&cfg, v, InitializerOptions{ConfigFlagName: "config"})
			cmd.RunE = func(*cobra.Command, []string) error {
				if cfg == nil || cfg.LogLevel != tc.want {
					t.Fatalf("configuration = %#v, want log level %s", cfg, tc.want)
				}
				return nil
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitializationFailureDoesNotRunOrReplaceConfig(t *testing.T) {
	t.Setenv("PANELTREE_LOG_LEVEL", "invalid")
	original := &Config{LogLevel: "warn"}
	cfg := original
	ran := false
	cmd := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
	cmd.Flags().String("config", "", "")
	cmd.PreRunE = NewInitializer(&cfg, viper.New(), InitializerOptions{ConfigFlagName: "config"})
	cmd.RunE = func(*cobra.Command, []string) error { ran = true; return nil }
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid configuration error")
	}
	if ran || cfg != original {
		t.Fatal("failed initialization ran the command or replaced configuration")
	}
}
