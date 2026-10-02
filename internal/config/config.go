// Package config resolves runtime settings independently of project artwork.
package config

import (
	"fmt"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/render"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config holds typed runtime settings. Treat it as read-only after initialization.
type Config struct {
	WebRoots     []string                `mapstructure:"web-roots"`
	Storage      storage.Config          `mapstructure:"storage"`
	Generation   render.GenerationConfig `mapstructure:"generation"`
	ComfyURL     string                  `mapstructure:"comfyui-url"`
	ComfyProfile string                  `mapstructure:"comfyui-profile"`
	MCPRoots     []string                `mapstructure:"mcp-roots"`
	LogLevel     string                  `mapstructure:"log-level"`
}

// InitializerOptions identifies the flag selecting the runtime configuration file.
type InitializerOptions struct {
	ConfigFlagName string
}

// NewInitializer fills the captured pointer only after successful resolution.
// Callers own the Viper instance; no package-global configuration is used.
func NewInitializer(dst **Config, v *viper.Viper, opts InitializerOptions) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		path, err := cmd.Flags().GetString(opts.ConfigFlagName)
		if err != nil {
			return fmt.Errorf("configuration flag: %w", err)
		}
		resolved, err := Resolve(v, path)
		if err != nil {
			return err
		}
		*dst = resolved
		return nil
	}
}

// Resolve applies flags > environment > explicit file > defaults.
// An absent path uses defaults; an explicitly missing file is an error.
func Resolve(v *viper.Viper, path string) (*Config, error) {
	v.SetDefault("log-level", "info")
	if err := v.BindEnv("log-level", "PANELTREE_LOG_LEVEL"); err != nil {
		return nil, fmt.Errorf("bind environment: %w", err)
	}
	if path != "" {
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read runtime configuration: %w", err)
		}
	}
	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return nil, fmt.Errorf("decode runtime configuration: %w", err)
	}
	if (cfg.ComfyURL == "") != (cfg.ComfyProfile == "") {
		return nil, fmt.Errorf("comfyui-url and comfyui-profile must be configured together")
	}
	if cfg.ComfyProfile != "" && !filepath.IsAbs(cfg.ComfyProfile) {
		cfg.ComfyProfile = filepath.Join(filepath.Dir(path), cfg.ComfyProfile)
	}
	if cfg.Storage.BlobRoot != "" && !filepath.IsAbs(cfg.Storage.BlobRoot) {
		cfg.Storage.BlobRoot = filepath.Join(filepath.Dir(path), cfg.Storage.BlobRoot)
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("invalid log-level %q: use debug, info, warn, or error", cfg.LogLevel)
	}
	return &cfg, nil
}
