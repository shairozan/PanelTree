// Package render defines leaf renderer contracts without selecting a backend.
package render

import (
	"context"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"image"
)

// Rasterizer reads an existing leaf into pixels. Returned images are read-only;
// placement, clipping, masks and opacity belong to the compositor.
type Rasterizer interface {
	Raster(context.Context, model.Source, string) (image.Image, error)
}

// SceneRasterizer receives the final assigned frame and desired leaf resolution.
// Existing PNG adapters can keep implementing only Rasterizer.
type SceneRasterizer interface {
	RasterScene(context.Context, Request) (image.Image, error)
}

// CacheRasterizer opts into deterministic production. CacheRecipe must describe
// every consumed dependency by content, algorithm/version and relevant context,
// never by machine path or time. Dependencies must remain frozen until render
// completes. A changed recipe is required for a new draft or stochastic seed.
type CacheRasterizer interface {
	Rasterizer
	SceneRasterizer
	CacheRecipe(context.Context, Request) (any, error)
}

type Descriptor struct {
	Name, Version string
	SourceKinds   []string
}
type Dependency struct{ Path, ContentHash string }
type Request struct {
	ImageInputs []ImageInput             `json:"image_inputs,omitempty"`
	Character   *model.ResolvedCharacter `json:"character,omitempty"`
	Generation  *Generation              `json:",omitempty"`
	Fit         string
	BaseDir     string
	Source      model.Source
	Scene       scene.Context
	Revision    model.Revision
}

// ImageInput is a frozen PNG, never a backend path or a textual reference.
type ImageInput struct {
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
	PNG    []byte `json:"png"`
}

// Generation is a backend-neutral request for a draft candidate, never a selection.
type Generation struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	Seed           uint64 `json:"seed"`
	Output         string `json:"output,omitempty"`
}
type Artifact struct {
	Path, ContentHash string
	Width, Height     int
	HasAlpha          bool
}
type Renderer interface {
	Descriptor() Descriptor
	Dependencies(Request) ([]Dependency, error)
	Render(context.Context, Request) (Artifact, error)
}
