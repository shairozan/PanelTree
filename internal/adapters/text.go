package adapters

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"image"
	"math"
	"strings"
)

type TextLine struct {
	Text     string
	X        []float64
	Baseline float64
}
type TextPlan struct {
	Lines                                   []TextLine
	Width, Height, MinimumWidth, LineHeight float64
}

func textFace(ctx context.Context, s model.Source, base string, scale float64) (font.Face, error) {
	if s.Font == "" || s.FontSize <= 0 || s.FontSize > 512 || math.IsNaN(s.FontSize) {
		return nil, fmt.Errorf("text requires explicit font and font_size in (0,512]")
	}
	b, err := ReadAsset(ctx, base, s.Font, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", s.Font, err)
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("font %s: %w", s.Font, err)
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: s.FontSize * scale, DPI: 72, Hinting: font.HintingNone})
}

// PlanText is the single wrapping/placement implementation used by measurement,
// rasterization and editable SVG export. It never chooses a system fallback font.
func PlanText(ctx context.Context, s model.Source, base string, width, height float64) (TextPlan, error) {
	var p TextPlan
	if err := ctx.Err(); err != nil {
		return p, err
	}
	if width <= 2 || height <= 2 || width > 8192 || height > 8192 || math.IsNaN(width) || math.IsNaN(height) {
		return p, fmt.Errorf("invalid text frame or overflow")
	}
	if len(s.Text) == 0 || len(s.Text) > 4096 {
		return p, fmt.Errorf("text requires 1..4096 basic Latin bytes")
	}
	for _, r := range s.Text {
		if r != '\n' && (r < 32 || r > 126) {
			return p, fmt.Errorf("unsupported text character U+%04X; basic Latin and newline only", r)
		}
	}
	face, err := textFace(ctx, s, base, 1)
	if err != nil {
		return p, err
	}
	defer func() { _ = face.Close() }()
	if _, err = ParseColor(s.Color); err != nil {
		return p, err
	}
	for _, r := range s.Text {
		if r != '\n' {
			if _, ok := face.GlyphAdvance(r); !ok {
				return p, fmt.Errorf("font %s has no glyph for %q", s.Font, r)
			}
		}
	}
	metrics := face.Metrics()
	p.LineHeight = float64(metrics.Height.Ceil())
	ascent := float64(metrics.Ascent.Ceil())
	if p.LineHeight <= 0 {
		return p, fmt.Errorf("invalid font metrics")
	}
	measure := func(text string) (float64, float64) {
		bounds, advance := font.BoundString(face, text)
		left := math.Min(0, float64(bounds.Min.X)/64)
		right := math.Max(float64(advance)/64, float64(bounds.Max.X)/64)
		return right - left + 2, 1 - left
	}
	lines := []string{}
	for _, paragraph := range strings.Split(s.Text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			wordWidth, _ := measure(word)
			p.MinimumWidth = math.Max(p.MinimumWidth, wordWidth)
			if wordWidth > width {
				return p, fmt.Errorf("text word %q overflows assigned width %.3f", word, width)
			}
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			cw, _ := measure(candidate)
			if cw > width {
				lines = append(lines, line)
				line = word
			} else {
				line = candidate
			}
		}
		lines = append(lines, line)
	}
	p.Height = float64(len(lines))*p.LineHeight + 2
	if p.Height > height {
		return p, fmt.Errorf("text height %.3f overflows frame %.3f", p.Height, height)
	}
	for i, line := range lines {
		w, x := measure(line)
		p.Width = math.Max(p.Width, w)
		baseline := 1 + ascent + float64(i)*p.LineHeight
		entry := TextLine{Text: line, Baseline: baseline}
		prev := rune(-1)
		for _, r := range line {
			if prev >= 0 {
				x += float64(face.Kern(prev, r)) / 64
			}
			bounds, advance, ok := face.GlyphBounds(r)
			if !ok {
				return p, fmt.Errorf("missing glyph %q", r)
			}
			if x+float64(bounds.Min.X)/64 < 0 || x+float64(bounds.Max.X)/64 > width || baseline+float64(bounds.Min.Y)/64 < 0 || baseline+float64(bounds.Max.Y)/64 > p.Height {
				return p, fmt.Errorf("glyph %q overflows measured frame", r)
			}
			entry.X = append(entry.X, x)
			x += float64(advance) / 64
			prev = r
		}
		p.Lines = append(p.Lines, entry)
	}
	return p, nil
}
func rasterText(ctx context.Context, r render.Request) (image.Image, error) {
	p, err := PlanText(ctx, r.Source, r.BaseDir, r.Scene.Bounds.Width, r.Scene.Bounds.Height)
	if err != nil {
		return nil, err
	}
	w, h, err := rasterSize(r.Scene.PixelSize.Width, r.Scene.PixelSize.Height)
	if err != nil {
		return nil, err
	}
	sx, sy := float64(w)/r.Scene.Bounds.Width, float64(h)/r.Scene.Bounds.Height
	// Uniform raster scale preserves glyph proportions. The compositor fits this
	// frame to its authored logical dimensions without reflowing text.
	scale := math.Max(sx, sy)
	w, h, err = rasterSize(r.Scene.Bounds.Width*scale, r.Scene.Bounds.Height*scale)
	if err != nil {
		return nil, err
	}
	face, err := textFace(ctx, r.Source, r.BaseDir, scale)
	if err != nil {
		return nil, err
	}
	defer func() { _ = face.Close() }()
	c, err := ParseColor(r.Source.Color)
	if err != nil {
		return nil, err
	}
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: im, Src: image.NewUniform(c), Face: face}
	for _, line := range p.Lines {
		for i, r := range line.Text {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			d.Dot = fixed.Point26_6{X: fixed.Int26_6(math.Round(line.X[i] * scale * 64)), Y: fixed.Int26_6(math.Round(line.Baseline * scale * 64))}
			d.DrawString(string(r))
		}
	}
	return im, nil
}
