// Package model defines the backend-neutral authoring schema.
package model

const Schema = "paneltree/v0.1"

type ID string
type Revision string
type Document struct {
	Schema  string   `yaml:"schema" json:"schema"`
	Book    *Book    `yaml:"book,omitempty" json:"book,omitempty"`
	Chapter *Chapter `yaml:"chapter,omitempty" json:"chapter,omitempty"`
	Page    *Page    `yaml:"page,omitempty" json:"page,omitempty"`
}
type Book struct {
	ID       ID       `yaml:"id" json:"id"`
	Title    string   `yaml:"title,omitempty" json:"title,omitempty"`
	Chapters []string `yaml:"chapters" json:"chapters"`
}
type Chapter struct {
	ID    ID       `yaml:"id" json:"id"`
	Title string   `yaml:"title,omitempty" json:"title,omitempty"`
	Pages []string `yaml:"pages" json:"pages"`
}
type Canvas struct {
	Width  float64 `yaml:"width" json:"width"`
	Height float64 `yaml:"height" json:"height"`
}
type Page struct {
	ID         ID      `yaml:"id" json:"id"`
	Canvas     Canvas  `yaml:"canvas" json:"canvas"`
	Background string  `yaml:"background,omitempty" json:"background,omitempty"`
	Layout     Layout  `yaml:"layout" json:"layout"`
	Panels     []Panel `yaml:"panels" json:"panels"`
}
type Size struct {
	Weight  *float64 `yaml:"weight,omitempty" json:"weight,omitempty"`
	Percent *float64 `yaml:"percent,omitempty" json:"percent,omitempty"`
}

// Layout is either a row/column container or a panel reference.
type Layout struct {
	Type     string   `yaml:"type,omitempty" json:"type,omitempty"`
	Size     *Size    `yaml:"size,omitempty" json:"size,omitempty"`
	Margin   float64  `yaml:"margin,omitempty" json:"margin,omitempty"`
	Gutter   float64  `yaml:"gutter,omitempty" json:"gutter,omitempty"`
	Panel    ID       `yaml:"panel,omitempty" json:"panel,omitempty"`
	Children []Layout `yaml:"children,omitempty" json:"children,omitempty"`
}
type Panel struct {
	ID     ID      `yaml:"id" json:"id"`
	Layers []Layer `yaml:"layers" json:"layers"`
}
type Frame struct {
	X      float64 `yaml:"x" json:"x"`
	Y      float64 `yaml:"y" json:"y"`
	Width  float64 `yaml:"width" json:"width"`
	Height float64 `yaml:"height" json:"height"`
}
type Transform struct {
	ScaleX   *float64 `yaml:"scale_x,omitempty" json:"scale_x,omitempty"`
	ScaleY   *float64 `yaml:"scale_y,omitempty" json:"scale_y,omitempty"`
	Rotation float64  `yaml:"rotation,omitempty" json:"rotation,omitempty"`
}

// Source describes content rather than a vendor workflow.
type Source struct {
	Draft    DraftRecipe `yaml:"draft,omitempty" json:"draft,omitempty"`
	Kind     string      `yaml:"kind" json:"kind"`
	Path     string      `yaml:"path,omitempty" json:"path,omitempty"`
	Text     string      `yaml:"text,omitempty" json:"text,omitempty"`
	Font     string      `yaml:"font,omitempty" json:"font,omitempty"`
	FontSize float64     `yaml:"font_size,omitempty" json:"font_size,omitempty"`
	Color    string      `yaml:"color,omitempty" json:"color,omitempty"`
}

// DraftRecipe changes only through an explicit authoring edit. Static adapters
// ignore it visually; generative adapters consume Seed and retain Revision in
// their production identity. Zero is a fixed value, never a randomize request.
type DraftRecipe struct {
	Revision uint64 `yaml:"revision,omitempty" json:"revision,omitempty"`
	Seed     uint64 `yaml:"seed,omitempty" json:"seed,omitempty"`
}
type Layer struct {
	ID        ID         `yaml:"id" json:"id"`
	Role      string     `yaml:"role,omitempty" json:"role,omitempty"`
	Source    *Source    `yaml:"source,omitempty" json:"source,omitempty"`
	Children  []Layer    `yaml:"children,omitempty" json:"children,omitempty"`
	Frame     *Frame     `yaml:"frame,omitempty" json:"frame,omitempty"`
	Fit       string     `yaml:"fit,omitempty" json:"fit,omitempty"`
	Transform *Transform `yaml:"transform,omitempty" json:"transform,omitempty"`
	Opacity   *float64   `yaml:"opacity,omitempty" json:"opacity,omitempty"`
	Mask      string     `yaml:"mask,omitempty" json:"mask,omitempty"`
}
type EditorialState string

const (
	Draft    EditorialState = "draft"
	Review   EditorialState = "review"
	Approved EditorialState = "approved"
)

type LockScope string

const (
	AssetLock     LockScope = "asset"
	PlacementLock LockScope = "placement"
	AllLock       LockScope = "all"
)
