package app

import (
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/render"
	"os"
	"time"
)

// NewRuntimeService configures optional generation without modifying story YAML.
// Paths and URLs are supplied by the host, never by MCP tool callers.
func NewRuntimeService(comfyURL, profilePath string, generation ...render.GenerationConfig) (*Service, error) {
	service := NewService()
	if len(generation) > 0 {
		cfg := generation[0]
		for name, profile := range cfg.Profiles {
			if name == "" {
				return nil, fmt.Errorf("empty generation profile name")
			}
			if err := adapters.ValidateIdeogramProfile(profile); err != nil {
				return nil, fmt.Errorf("profile %s: %w", name, err)
			}
		}
		if cfg.DefaultProfile != "" {
			if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
				return nil, fmt.Errorf("unknown default generation profile")
			}
		}
		if len(cfg.Profiles) > 0 {
			c := cfg.Providers.Ideogram
			if c.APIKeyEnv == "" {
				c.APIKeyEnv = "IDEOGRAM_API_KEY"
			}
			if c.Concurrency == 0 {
				c.Concurrency = 2
			}
			if c.Concurrency < 1 || c.Concurrency > 8 {
				return nil, fmt.Errorf("ideogram concurrency must be 1 through 8")
			}
			if c.Timeout == "" {
				c.Timeout = "5m"
			}
			timeout, err := time.ParseDuration(c.Timeout)
			if err != nil || timeout <= 0 {
				return nil, fmt.Errorf("invalid Ideogram timeout")
			}
			service.ideogram = &adapters.Ideogram{APIKey: os.Getenv(c.APIKeyEnv), Timeout: timeout, Slots: make(chan struct{}, c.Concurrency)}
		}
		service.generation = cfg
	}
	if comfyURL == "" && profilePath == "" {
		return service, nil
	}
	if e := adapters.ValidateComfyURL(comfyURL); e != nil {
		return nil, e
	}
	profile, e := adapters.LoadComfyProfile(profilePath)
	if e != nil {
		return nil, e
	}
	service.comfy = &adapters.ComfyUI{URL: comfyURL, Profile: profile}
	return service, nil
}
