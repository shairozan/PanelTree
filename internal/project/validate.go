package project

import (
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"math"
	"regexp"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func validID(id model.ID) bool { return idPattern.MatchString(string(id)) }
func finite(v float64) bool    { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func positive(v float64) bool  { return finite(v) && v > 0 }
func validatePage(path string, p model.Page, n *yaml.Node) error {
	if !positive(p.Canvas.Width) || !positive(p.Canvas.Height) {
		return diagnostic(path, field(n, "canvas"), "canvas width and height must be positive finite numbers")
	}
	panels := map[model.ID]bool{}
	for i, panel := range p.Panels {
		pn := field(n, "panels").Content[i]
		if !validID(panel.ID) || panels[panel.ID] {
			return diagnostic(path, field(pn, "id"), "invalid or duplicate panel id %q", panel.ID)
		}
		panels[panel.ID] = true
		if err := validateLayers(path, panel.Layers, field(pn, "layers"), map[model.ID]bool{}); err != nil {
			return err
		}
	}
	used := map[model.ID]bool{}
	if err := validateLayout(path, p.Layout, field(n, "layout"), panels, used); err != nil {
		return err
	}
	for id := range panels {
		if !used[id] {
			return diagnostic(path, field(n, "layout"), "panel %q is not placed", id)
		}
	}
	return nil
}
func validateLayout(path string, l model.Layout, n *yaml.Node, panels, used map[model.ID]bool) error {
	if !finite(l.Margin) || l.Margin < 0 || !finite(l.Gutter) || l.Gutter < 0 {
		return diagnostic(path, n, "margin/gutter must be finite and nonnegative")
	}
	if l.Size != nil {
		s := l.Size
		if (s.Weight == nil) == (s.Percent == nil) {
			return diagnostic(path, field(n, "size"), "size requires exactly one weight or percent")
		}
		if s.Weight != nil && !positive(*s.Weight) {
			return diagnostic(path, n, "weight must be positive")
		}
		if s.Percent != nil && (!positive(*s.Percent) || *s.Percent > 100) {
			return diagnostic(path, n, "percent must be in (0,100]")
		}
	}
	if l.Panel != "" {
		if l.Type != "" || len(l.Children) > 0 || l.Margin != 0 || l.Gutter != 0 {
			return diagnostic(path, n, "panel reference cannot also be a layout container")
		}
		if !panels[l.Panel] {
			return diagnostic(path, field(n, "panel"), "missing panel %q", l.Panel)
		}
		if used[l.Panel] {
			return diagnostic(path, field(n, "panel"), "duplicate panel placement %q", l.Panel)
		}
		used[l.Panel] = true
		return nil
	}
	if (l.Type != "row" && l.Type != "column") || len(l.Children) == 0 {
		return diagnostic(path, n, "layout requires row/column with children or a panel reference")
	}
	for i, c := range l.Children {
		if err := validateLayout(path, c, field(n, "children").Content[i], panels, used); err != nil {
			return err
		}
	}
	return nil
}
func validateLayers(path string, layers []model.Layer, n *yaml.Node, ids map[model.ID]bool) error {
	for i, l := range layers {
		ln := n.Content[i]
		if !validID(l.ID) || ids[l.ID] {
			return diagnostic(path, field(ln, "id"), "invalid or duplicate layer id %q", l.ID)
		}
		ids[l.ID] = true
		if (l.Source == nil) == (len(l.Children) == 0) {
			return diagnostic(path, ln, "layer needs exactly one source or nonempty children")
		}
		if l.Source != nil {
			if err := validateSource(path, *l.Source, field(ln, "source")); err != nil {
				return err
			}
		}
		if l.Fit != "" && l.Fit != "contain" && l.Fit != "cover" {
			return diagnostic(path, ln, "fit must be contain or cover")
		}
		if l.Frame != nil {
			f := l.Frame
			if !finite(f.X) || !finite(f.Y) || !positive(f.Width) || !positive(f.Height) {
				return diagnostic(path, field(ln, "frame"), "invalid frame")
			}
		}
		if l.Opacity != nil && (!finite(*l.Opacity) || *l.Opacity < 0 || *l.Opacity > 1) {
			return diagnostic(path, ln, "opacity must be in [0,1]")
		}
		if l.Transform != nil {
			t := l.Transform
			if !finite(t.Rotation) || (t.ScaleX != nil && !positive(*t.ScaleX)) || (t.ScaleY != nil && !positive(*t.ScaleY)) {
				return diagnostic(path, ln, "invalid transform")
			}
		}
		if err := validateLayers(path, l.Children, field(ln, "children"), ids); err != nil {
			return err
		}
	}
	return nil
}
func validateSource(path string, s model.Source, n *yaml.Node) error {
	switch s.Kind {
	case "image", "svg":
		if s.Path == "" || s.Text != "" || s.Font != "" || s.FontSize != 0 || s.Color != "" {
			return diagnostic(path, n, "image/svg source requires path and no text fields")
		}
	case "text":
		if s.Text == "" || s.Font == "" || !positive(s.FontSize) || s.Path != "" {
			return diagnostic(path, n, "text source requires text, font and positive font_size, without path")
		}
	default:
		return diagnostic(path, n, "unsupported source kind %q", s.Kind)
	}
	return nil
}
