// Package scene defines logical geometry and detached resolved scenes.
package scene

import (
	"context"
	"github.com/shairozan/PanelTree/model"
)

type Rect struct{ X, Y, Width, Height float64 }
type Constraints struct{ Min, Max model.Canvas }
type Metrics struct{ Minimum, Preferred model.Canvas }

// MeasureRequest contains a value copy of a leaf and its assigned logical constraints.
// Providers read already-available metadata/font metrics; they must not generate assets.
type MeasureRequest struct {
	Source      model.Source
	BaseDir     string
	Constraints Constraints
}
type Measurer interface {
	Measure(context.Context, MeasureRequest) (Metrics, error)
}
type Context struct {
	Bounds    Rect
	PixelSize model.Canvas
}
type Node struct {
	ID       model.ID
	Bounds   Rect
	Children []Node
	Source   *model.Source
}
type Component interface {
	ID() model.ID
	Measure(Context, Constraints) (Metrics, error)
	Layout(Context, Rect) (Node, error)
}
