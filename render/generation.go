package render

// GenerationConfig is host runtime configuration, separate from project artwork.
type GenerationConfig struct {
	DefaultProfile string `mapstructure:"default-profile"`
	Providers      struct {
		Ideogram IdeogramConfig `mapstructure:"ideogram"`
	} `mapstructure:"providers"`
	Profiles map[string]GenerationProfile `mapstructure:"profiles"`
}
type IdeogramConfig struct {
	APIKeyEnv   string `mapstructure:"api-key-env"`
	Concurrency int    `mapstructure:"concurrency"`
	Timeout     string `mapstructure:"timeout"`
}
type GenerationProfile struct {
	Quality     string `mapstructure:"quality" json:"quality,omitempty"`
	Size        string `mapstructure:"size" json:"size,omitempty"`
	Renderer    string `mapstructure:"renderer" json:"renderer"`
	Model       string `mapstructure:"model" json:"model"`
	Operation   string `mapstructure:"operation" json:"operation"`
	Speed       string `mapstructure:"rendering-speed" json:"rendering_speed"`
	MagicPrompt string `mapstructure:"magic-prompt" json:"magic_prompt"`
	StyleType   string `mapstructure:"style-type" json:"style_type"`
}
