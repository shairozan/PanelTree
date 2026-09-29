// Package scene defines layout contracts; resolution is implemented in Sprint 02.
package scene

import "github.com/shairozan/PanelTree/model"

type Rect struct{ X, Y, Width, Height float64 }
type Constraints struct{ Min, Max model.Canvas }
type Metrics struct{ Minimum, Preferred model.Canvas }
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
