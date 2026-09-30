package app

import "github.com/shairozan/PanelTree/internal/adapters"

// NewRuntimeService configures optional generation without modifying story YAML.
// Paths and URLs are supplied by the host, never by MCP tool callers.
func NewRuntimeService(comfyURL, profilePath string) (*Service, error) {
	if comfyURL == "" && profilePath == "" {
		return NewService(), nil
	}
	if e := adapters.ValidateComfyURL(comfyURL); e != nil {
		return nil, e
	}
	profile, e := adapters.LoadComfyProfile(profilePath)
	if e != nil {
		return nil, e
	}
	return NewService(WithComfyUI(&adapters.ComfyUI{URL: comfyURL, Profile: profile})), nil
}
