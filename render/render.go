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

type Descriptor struct {
	Name, Version string
	SourceKinds   []string
}
type Dependency struct{ Path, ContentHash string }
type Request struct {
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
