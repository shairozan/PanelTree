package model

// CharacterUse refers to a package relative to the owning project directory.
// Descriptions live in the package, not in each panel.
type CharacterUse struct {
	Package    string   `yaml:"package" json:"package"`
	Costume    string   `yaml:"costume,omitempty" json:"costume,omitempty"`
	Expression string   `yaml:"expression,omitempty" json:"expression,omitempty"`
	Pose       string   `yaml:"pose,omitempty" json:"pose,omitempty"`
	Props      []string `yaml:"props,omitempty" json:"props,omitempty"`
}

type CharacterPackage struct {
	Schema      string               `yaml:"schema" json:"schema"`
	ID          string               `yaml:"id" json:"id"`
	Version     string               `yaml:"version" json:"version"`
	Description string               `yaml:"description" json:"description"`
	Palette     []string             `yaml:"palette,omitempty" json:"palette,omitempty"`
	References  []CharacterReference `yaml:"references,omitempty" json:"references,omitempty"`
	Costumes    []CharacterState     `yaml:"costumes,omitempty" json:"costumes,omitempty"`
	Expressions []CharacterState     `yaml:"expressions,omitempty" json:"expressions,omitempty"`
	Poses       []CharacterState     `yaml:"poses,omitempty" json:"poses,omitempty"`
	Props       []CharacterReference `yaml:"props,omitempty" json:"props,omitempty"`
}
type CharacterState struct {
	ID          string               `yaml:"id" json:"id"`
	Version     string               `yaml:"version" json:"version"`
	Description string               `yaml:"description" json:"description"`
	References  []CharacterReference `yaml:"references,omitempty" json:"references,omitempty"`
}
type CharacterReference struct {
	ID          string `yaml:"id" json:"id"`
	Version     string `yaml:"version" json:"version"`
	Path        string `yaml:"path" json:"path"`
	License     string `yaml:"license" json:"license"`
	Attribution string `yaml:"attribution" json:"attribution"`
	Description string `yaml:"description" json:"description"`
}
type ResolvedReference struct {
	CharacterReference
	SHA256 string `json:"sha256"`
}

// ResolvedCharacter is a frozen, selected dependency set. Unselected states are
// deliberately absent, so unrelated changes do not invalidate a candidate.
type ResolvedCharacter struct {
	Use         CharacterUse        `json:"use"`
	ID          string              `json:"id"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Palette     []string            `json:"palette,omitempty"`
	Costume     *CharacterState     `json:"costume,omitempty"`
	Expression  *CharacterState     `json:"expression,omitempty"`
	Pose        *CharacterState     `json:"pose,omitempty"`
	References  []ResolvedReference `json:"references,omitempty"`
	Props       []ResolvedReference `json:"props,omitempty"`
}
