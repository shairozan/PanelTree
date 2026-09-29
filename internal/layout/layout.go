// Package layout measures and resolves page composition without rendering.
package layout

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"math"
)

type MeasureRequest = scene.MeasureRequest
type Measurer = scene.Measurer
type Options struct {
	Width, Height int
	Fit           string
	BaseDir       string
	Measurer      Measurer
}
type engine struct {
	ctx     context.Context
	options Options
	panels  map[model.ID]model.Panel
	used    map[model.ID]bool
}

// measured is the private first-pass tree. No source mutations are made.
type measured struct {
	node     scene.ResolvedNode
	children []measured
}

func Resolve(ctx context.Context, page model.Page, opts Options) (scene.Resolved, error) {
	if err := ctx.Err(); err != nil {
		return scene.Resolved{}, err
	}
	if !positive(page.Canvas.Width) || !positive(page.Canvas.Height) {
		return scene.Resolved{}, fmt.Errorf("page %s: invalid canvas", page.ID)
	}
	output, err := resolveOutput(page.Canvas, opts)
	if err != nil {
		return scene.Resolved{}, err
	}
	e := engine{ctx: ctx, options: opts, panels: map[model.ID]model.Panel{}, used: map[model.ID]bool{}}
	for _, p := range page.Panels {
		if p.ID == "" {
			return scene.Resolved{}, fmt.Errorf("empty panel id")
		}
		if _, ok := e.panels[p.ID]; ok {
			return scene.Resolved{}, fmt.Errorf("duplicate panel %s", p.ID)
		}
		e.panels[p.ID] = p
	}
	bounds := scene.Rect{Width: page.Canvas.Width, Height: page.Canvas.Height}
	// Measure recursively using the explicit track allocations as width constraints.
	plan, err := e.measureLayout(page.Layout, bounds, 0)
	if err != nil {
		return scene.Resolved{}, fmt.Errorf("page %s: %w", page.ID, err)
	}
	for id := range e.panels {
		if !e.used[id] {
			return scene.Resolved{}, fmt.Errorf("unplaced panel %s", id)
		}
	}
	root := measured{node: scene.ResolvedNode{ID: string(page.ID), Kind: "page", Bounds: bounds, Opacity: 1}, children: []measured{plan}}
	resolved, err := e.arrange(root, scene.Identity(), bounds, output, 0)
	if err != nil {
		return scene.Resolved{}, err
	}
	return scene.NewResolved(resolved, output), nil
}

func finite(v float64) bool   { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func positive(v float64) bool { return finite(v) && v > 0 }
func resolveOutput(canvas model.Canvas, o Options) (scene.Output, error) {
	fit := o.Fit
	if fit == "" {
		fit = "error"
	}
	if fit != "error" && fit != "contain" && fit != "cover" {
		return scene.Output{}, fmt.Errorf("invalid output fit %q", fit)
	}
	native := o.Width == 0 && o.Height == 0
	if native {
		if canvas.Width > 1e7 || canvas.Height > 1e7 {
			return scene.Output{}, fmt.Errorf("default output dimension exceeds 10000000")
		}
		o.Width, o.Height = int(math.Round(canvas.Width)), int(math.Round(canvas.Height))
	}
	if o.Width <= 0 || o.Height <= 0 || o.Width > 1e7 || o.Height > 1e7 {
		return scene.Output{}, fmt.Errorf("output requires width and height in [1,10000000]")
	}
	if native {
		return scene.Output{Width: o.Width, Height: o.Height, Fit: fit, World: scene.Identity(), Clip: scene.PixelRect{Right: o.Width, Bottom: o.Height}}, nil
	}
	sx, sy := float64(o.Width)/canvas.Width, float64(o.Height)/canvas.Height
	if fit == "error" && math.Abs(sx-sy) > 1e-9*math.Max(sx, sy) {
		return scene.Output{}, fmt.Errorf("output aspect ratio differs; select contain or cover")
	}
	scale := sx
	if fit == "contain" {
		scale = math.Min(sx, sy)
	}
	if fit == "cover" {
		scale = math.Max(sx, sy)
	}
	tx, ty := (float64(o.Width)-canvas.Width*scale)/2, (float64(o.Height)-canvas.Height*scale)/2
	return scene.Output{Width: o.Width, Height: o.Height, Fit: fit, World: scene.Matrix{scale, 0, 0, scale, tx, ty}, Clip: scene.PixelRect{Right: o.Width, Bottom: o.Height}}, nil
}

func (e *engine) measureLayout(l model.Layout, b scene.Rect, depth int) (measured, error) {
	if err := e.ctx.Err(); err != nil {
		return measured{}, err
	}
	if depth > 64 {
		return measured{}, fmt.Errorf("layout nesting exceeds 64")
	}
	if !positive(b.Width) || !positive(b.Height) {
		return measured{}, fmt.Errorf("nonpositive allocation")
	}
	n := measured{node: scene.ResolvedNode{Kind: l.Type, Bounds: b, Opacity: 1}}
	if l.Panel != "" {
		if l.Type != "" || len(l.Children) > 0 || l.Margin != 0 || l.Gutter != 0 {
			return measured{}, fmt.Errorf("panel %s also defines a container", l.Panel)
		}
		p, ok := e.panels[l.Panel]
		if !ok {
			return measured{}, fmt.Errorf("missing panel %s", l.Panel)
		}
		if e.used[l.Panel] {
			return measured{}, fmt.Errorf("duplicate panel %s", l.Panel)
		}
		e.used[l.Panel] = true
		n.node.ID, n.node.Kind = string(p.ID), "panel"
		ids := map[model.ID]bool{}
		for _, layer := range p.Layers {
			child, err := e.measureLayer(layer, model.Canvas{Width: b.Width, Height: b.Height}, ids, 0)
			if err != nil {
				return measured{}, fmt.Errorf("panel %s: %w", p.ID, err)
			}
			n.children = append(n.children, child)
		}
		return n, nil
	}
	if (l.Type != "row" && l.Type != "column") || len(l.Children) == 0 {
		return measured{}, fmt.Errorf("layout requires a row/column with children")
	}
	if !finite(l.Margin) || l.Margin < 0 || !finite(l.Gutter) || l.Gutter < 0 {
		return measured{}, fmt.Errorf("invalid margin or gutter")
	}
	width, height := b.Width-2*l.Margin, b.Height-2*l.Margin
	extent := width
	if l.Type == "column" {
		extent = height
	}
	available := extent - float64(len(l.Children)-1)*l.Gutter
	if !positive(width) || !positive(height) || !positive(available) {
		return measured{}, fmt.Errorf("margins/gutters exhaust container")
	}
	tracks, err := allocate(available, l.Children)
	if err != nil {
		return measured{}, err
	}
	cursor := l.Margin
	for i, c := range l.Children {
		cb := scene.Rect{X: l.Margin, Y: l.Margin, Width: width, Height: height}
		if l.Type == "row" {
			cb.X = cursor
			cb.Width = tracks[i]
		} else {
			cb.Y = cursor
			cb.Height = tracks[i]
		}
		child, err := e.measureLayout(c, cb, depth+1)
		if err != nil {
			return measured{}, fmt.Errorf("%s child %d: %w", l.Type, i, err)
		}
		n.children = append(n.children, child)
		cursor += tracks[i] + l.Gutter
	}
	return n, nil
}

func allocate(available float64, children []model.Layout) ([]float64, error) {
	weights := make([]float64, len(children))
	percents := make([]float64, len(children))
	sumPercent, sumWeight := 0.0, 0.0
	for i, c := range children {
		if c.Size == nil {
			weights[i] = 1
		} else {
			s := c.Size
			if (s.Weight == nil) == (s.Percent == nil) {
				return nil, fmt.Errorf("size must specify exactly one weight or percent")
			}
			if s.Weight != nil {
				if !positive(*s.Weight) {
					return nil, fmt.Errorf("invalid weight")
				}
				weights[i] = *s.Weight
			} else {
				if !positive(*s.Percent) || *s.Percent > 100 {
					return nil, fmt.Errorf("invalid percent")
				}
				percents[i] = *s.Percent
			}
		}
		sumPercent += percents[i]
		sumWeight += weights[i]
	}
	if !finite(sumWeight) || sumPercent > 100+1e-9 {
		return nil, fmt.Errorf("track percentages exceed 100 or weights overflow")
	}
	if sumWeight == 0 && math.Abs(sumPercent-100) > 1e-9 {
		return nil, fmt.Errorf("percent-only tracks must total 100")
	}
	remaining := available * (1 - sumPercent/100)
	if sumWeight > 0 && !positive(remaining) {
		return nil, fmt.Errorf("no space remains for weighted tracks")
	}
	result := make([]float64, len(children))
	used := 0.0
	for i := range children {
		v := available * percents[i] / 100
		if weights[i] > 0 {
			v = remaining * (weights[i] / sumWeight)
		}
		if i == len(children)-1 {
			v = available - used
		}
		if !positive(v) {
			return nil, fmt.Errorf("track %d has nonpositive extent", i)
		}
		result[i] = v
		used += v
	}
	return result, nil
}

func (e *engine) measureLayer(l model.Layer, parent model.Canvas, ids map[model.ID]bool, depth int) (measured, error) {
	if err := e.ctx.Err(); err != nil {
		return measured{}, err
	}
	if depth > 64 {
		return measured{}, fmt.Errorf("layer nesting exceeds 64")
	}
	if l.ID == "" || ids[l.ID] {
		return measured{}, fmt.Errorf("invalid or duplicate layer id %q", l.ID)
	}
	ids[l.ID] = true
	if (l.Source == nil) == (len(l.Children) == 0) {
		return measured{}, fmt.Errorf("layer %s requires a source or children", l.ID)
	}
	f := model.Frame{Width: 1, Height: 1}
	if l.Frame != nil {
		f = *l.Frame
	}
	if !finite(f.X) || !finite(f.Y) || !positive(f.Width) || !positive(f.Height) {
		return measured{}, fmt.Errorf("layer %s: invalid frame", l.ID)
	}
	b := scene.Rect{X: f.X * parent.Width, Y: f.Y * parent.Height, Width: f.Width * parent.Width, Height: f.Height * parent.Height}
	if !finite(b.X) || !finite(b.Y) || !positive(b.Width) || !positive(b.Height) {
		return measured{}, fmt.Errorf("layer %s: frame overflow", l.ID)
	}
	opacity := 1.0
	if l.Opacity != nil {
		opacity = *l.Opacity
	}
	if !finite(opacity) || opacity < 0 || opacity > 1 {
		return measured{}, fmt.Errorf("layer %s: invalid opacity", l.ID)
	}
	if l.Fit != "" && l.Fit != "contain" && l.Fit != "cover" {
		return measured{}, fmt.Errorf("layer %s: invalid fit", l.ID)
	}
	local, err := layerMatrix(b, l.Transform)
	if err != nil {
		return measured{}, fmt.Errorf("layer %s: %w", l.ID, err)
	}
	n := measured{node: scene.ResolvedNode{ID: string(l.ID), Kind: "layer", Bounds: b, World: local, Role: l.Role, Fit: l.Fit, Mask: l.Mask, Opacity: opacity}}
	if l.Source != nil {
		s := *l.Source
		n.node.Source = &s
		if e.options.Measurer == nil {
			if s.Kind == "text" {
				return measured{}, fmt.Errorf("layer %s: text requires a measurement provider", l.ID)
			}
		} else {
			metrics, err := e.options.Measurer.Measure(e.ctx, MeasureRequest{Source: s, BaseDir: e.options.BaseDir, Constraints: scene.Constraints{Max: model.Canvas{Width: b.Width, Height: b.Height}}})
			if err != nil {
				return measured{}, fmt.Errorf("layer %s measurement: %w", l.ID, err)
			}
			if err := validateMetrics(metrics, b, s.Kind); err != nil {
				return measured{}, fmt.Errorf("layer %s: %w", l.ID, err)
			}
			n.node.Metrics, n.node.Measured = metrics, true
		}
	}
	for _, c := range l.Children {
		child, err := e.measureLayer(c, model.Canvas{Width: b.Width, Height: b.Height}, ids, depth+1)
		if err != nil {
			return measured{}, err
		}
		n.children = append(n.children, child)
	}
	return n, nil
}
func layerMatrix(b scene.Rect, t *model.Transform) (scene.Matrix, error) {
	sx, sy, rotation := 1.0, 1.0, 0.0
	if t != nil {
		if t.ScaleX != nil {
			sx = *t.ScaleX
		}
		if t.ScaleY != nil {
			sy = *t.ScaleY
		}
		rotation = t.Rotation
	}
	if !positive(sx) || !positive(sy) || !finite(rotation) {
		return scene.Matrix{}, fmt.Errorf("invalid transform")
	}
	return scene.Translate(b.X, b.Y).Multiply(scene.Translate(b.Width/2, b.Height/2)).Multiply(scene.Rotate(math.Mod(rotation, 360))).Multiply(scene.Scale(sx, sy)).Multiply(scene.Translate(-b.Width/2, -b.Height/2)), nil
}
func validateMetrics(m scene.Metrics, b scene.Rect, kind string) error {
	for _, v := range []float64{m.Minimum.Width, m.Minimum.Height, m.Preferred.Width, m.Preferred.Height} {
		if !finite(v) || v < 0 {
			return fmt.Errorf("invalid measurement metrics")
		}
	}
	if m.Minimum.Width > b.Width+1e-9 || m.Minimum.Height > b.Height+1e-9 {
		return fmt.Errorf("minimum size exceeds allocation")
	}
	if kind == "text" && (m.Preferred.Width > b.Width+1e-9 || m.Preferred.Height > b.Height+1e-9) {
		return fmt.Errorf("text overflows assigned frame")
	}
	return nil
}

func (e *engine) arrange(p measured, parent scene.Matrix, clip scene.Rect, output scene.Output, depth int) (scene.ResolvedNode, error) {
	if err := e.ctx.Err(); err != nil {
		return scene.ResolvedNode{}, err
	}
	n := p.node
	local := scene.Translate(n.Bounds.X, n.Bounds.Y)
	if n.Kind == "layer" {
		local = n.World
	}
	n.World = parent.Multiply(local)
	n.LogicalBounds = n.World.Bounds(n.Bounds.Width, n.Bounds.Height)
	if n.Kind == "panel" {
		clip = n.LogicalBounds
	}
	n.Clip = clip
	pixels := output.World.Multiply(n.World).Bounds(n.Bounds.Width, n.Bounds.Height)
	edges := []float64{pixels.X, pixels.Y, pixels.X + pixels.Width, pixels.Y + pixels.Height}
	for _, v := range edges {
		if !finite(v) || math.Abs(v) > 1e9 {
			return scene.ResolvedNode{}, fmt.Errorf("node %s: transformed pixel bounds exceed supported range", n.ID)
		}
	}
	n.Pixels = scene.PixelRect{Left: pixelEdge(edges[0]), Top: pixelEdge(edges[1]), Right: pixelEdge(edges[2]), Bottom: pixelEdge(edges[3])}
	for _, c := range p.children {
		child, err := e.arrange(c, n.World, clip, output, depth+1)
		if err != nil {
			return scene.ResolvedNode{}, err
		}
		n.Children = append(n.Children, child)
	}
	return n, nil
}

// Algebraically identical edges can straddle a half pixel after floating-point
// matrix composition. Snap numerical noise at that threshold before rounding.
func pixelEdge(v float64) int {
	half := math.Round(v*2) / 2
	if math.Abs(v-half) <= 1e-12*math.Max(1, math.Abs(v)) {
		v = half
	}
	return int(math.Round(v))
}
