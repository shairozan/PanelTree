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

type Descriptor struct {
	Name, Version string
	SourceKinds   []string
}
type Dependency struct{ Path, ContentHash string }
type Request struct {
	Fit      string
	BaseDir  string
	Source   model.Source
	Scene    scene.Context
	Revision model.Revision
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
