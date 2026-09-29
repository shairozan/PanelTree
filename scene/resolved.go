package scene

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/model"
	"math"
)

// PixelRect uses half-open edges rather than independently rounded widths.
type PixelRect struct{ Left, Top, Right, Bottom int }
type Matrix [6]float64
type ResolvedNode struct {
	ID            string
	Kind          string
	Bounds        Rect
	World         Matrix
	LogicalBounds Rect
	Pixels        PixelRect
	Clip          Rect
	Metrics       Metrics
	Source        *model.Source
	Role          string
	Fit           string
	Mask          string
	Opacity       float64
	Measured      bool
	Children      []ResolvedNode
}
type Output struct {
	Width, Height int
	Fit           string
	World         Matrix
	Clip          PixelRect
}

// Resolved owns a detached tree. Accessors return copies, not mutable backing slices.
type Resolved struct {
	root   ResolvedNode
	output Output
}

func NewResolved(root ResolvedNode, output Output) Resolved {
	return Resolved{root: clone(root), output: output}
}
func (r Resolved) Tree() ResolvedNode { return clone(r.root) }
func (r Resolved) Output() Output     { return r.output }
func (r Resolved) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Root   ResolvedNode `json:"root"`
		Output Output       `json:"output"`
	}{r.root, r.output})
}
func clone(n ResolvedNode) ResolvedNode {
	if n.Source != nil {
		s := *n.Source
		n.Source = &s
	}
	if n.Children != nil {
		children := make([]ResolvedNode, len(n.Children))
		for i, c := range n.Children {
			children[i] = clone(c)
		}
		n.Children = children
	}
	return n
}

// Matrix is [a,b,c,d,e,f]: x'=a*x+c*y+e; y'=b*x+d*y+f.
func Identity() Matrix { return Matrix{1, 0, 0, 1, 0, 0} }

// Multiply applies b first, then a.
func (a Matrix) Multiply(b Matrix) Matrix {
	return Matrix{a[0]*b[0] + a[2]*b[1], a[1]*b[0] + a[3]*b[1], a[0]*b[2] + a[2]*b[3], a[1]*b[2] + a[3]*b[3], a[0]*b[4] + a[2]*b[5] + a[4], a[1]*b[4] + a[3]*b[5] + a[5]}
}
func Translate(x, y float64) Matrix { return Matrix{1, 0, 0, 1, x, y} }
func Scale(x, y float64) Matrix     { return Matrix{x, 0, 0, y, 0, 0} }
func Rotate(degrees float64) Matrix {
	r := degrees * math.Pi / 180
	s, c := math.Sincos(r)
	return Matrix{c, s, -s, c, 0, 0}
}
func (m Matrix) Point(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}
func (m Matrix) Bounds(w, h float64) Rect {
	x0, y0 := m.Point(0, 0)
	x1, y1 := m.Point(w, 0)
	x2, y2 := m.Point(0, h)
	x3, y3 := m.Point(w, h)
	left, right := math.Min(math.Min(x0, x1), math.Min(x2, x3)), math.Max(math.Max(x0, x1), math.Max(x2, x3))
	top, bottom := math.Min(math.Min(y0, y1), math.Min(y2, y3)), math.Max(math.Max(y0, y1), math.Max(y2, y3))
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}
