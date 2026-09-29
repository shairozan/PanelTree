// Package compose composites resolved scenes using injected leaf rasterizers.
package compose

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"image/draw"
	"math"
)

func Page(ctx context.Context, resolved scene.Resolved, base string, background color.Color, raster render.Rasterizer) (*image.RGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o := resolved.Output()
	if o.Width <= 0 || o.Height <= 0 || o.Width > 8192 || o.Height > 8192 || int64(o.Width)*int64(o.Height) > 16<<20 {
		return nil, fmt.Errorf("output exceeds 8192 per axis or 16777216 pixels")
	}
	if raster == nil {
		return nil, fmt.Errorf("missing rasterizer")
	}
	e := engine{ctx: ctx, base: base, raster: raster, output: o, rect: image.Rect(0, 0, o.Width, o.Height)}
	out, err := e.surface()
	if err != nil {
		return nil, err
	}
	if background != nil {
		draw.Draw(out, out.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	}
	if err = e.paint(out, resolved.Tree(), 0); err != nil {
		return nil, err
	}
	return out, nil
}

// Bound simultaneously live compositing surfaces to 256 MiB. The PNG adapter
// separately limits each decoded input to 4M pixels (including masks).
type engine struct {
	ctx    context.Context
	base   string
	raster render.Rasterizer
	output scene.Output
	rect   image.Rectangle
	live   int64
	nodes  int
}

func (e *engine) surface() (*image.RGBA, error) {
	n := int64(e.rect.Dx()) * int64(e.rect.Dy()) * 4
	if e.live+n > 256<<20 {
		return nil, fmt.Errorf("composition exceeds 256 MiB surface budget")
	}
	e.live += n
	return image.NewRGBA(e.rect), nil
}
func (e *engine) release() { e.live -= int64(e.rect.Dx()) * int64(e.rect.Dy()) * 4 }
func inverse(m scene.Matrix) (scene.Matrix, error) {
	d := m[0]*m[3] - m[1]*m[2]
	if d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return scene.Matrix{}, fmt.Errorf("singular transform")
	}
	r := scene.Matrix{m[3] / d, -m[1] / d, -m[2] / d, m[0] / d, 0, 0}
	r[4], r[5] = r.Point(-m[4], -m[5])
	for _, v := range r {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return scene.Matrix{}, fmt.Errorf("invalid transform")
		}
	}
	return r, nil
}
func (e *engine) paint(dst *image.RGBA, n scene.ResolvedNode, depth int) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	e.nodes++
	if depth > 128 || e.nodes > 4096 {
		return fmt.Errorf("composition tree exceeds depth/node limit")
	}
	if n.Kind != "layer" {
		for _, c := range n.Children {
			if err := e.paint(dst, c, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	layer, err := e.surface()
	if err != nil {
		return err
	}
	defer e.release()
	world := e.output.World.Multiply(n.World)
	footprint := pixelRect(world.Bounds(n.Bounds.Width, n.Bounds.Height))
	inv, err := inverse(world)
	if err != nil {
		return fmt.Errorf("layer %s: %w", n.ID, err)
	}
	if n.Source != nil {
		src, err := e.rasterLeaf(n, world)
		if err != nil {
			return fmt.Errorf("layer %s: %w", n.ID, err)
		}
		if src == nil || src.Bounds().Empty() {
			return fmt.Errorf("layer %s: empty raster", n.ID)
		}
		sw, sh := float64(src.Bounds().Dx()), float64(src.Bounds().Dy())
		scale := math.Min(n.Bounds.Width/sw, n.Bounds.Height/sh)
		if n.Fit == "cover" {
			scale = math.Max(n.Bounds.Width/sw, n.Bounds.Height/sh)
		}
		ox, oy := (n.Bounds.Width-sw*scale)/2, (n.Bounds.Height-sh*scale)/2
		// Clip's origin is in logical page coordinates, not node coordinates.
		cm := e.output.World.Multiply(scene.Translate(n.Clip.X, n.Clip.Y))
		clip := pixelRect(cm.Bounds(n.Clip.Width, n.Clip.Height)).Intersect(e.rect)
		// Rounded half-open bounds decide pixel ownership at half-pixel ties;
		// inverse sampling below only decides coverage within that footprint.
		clip = clip.Intersect(footprint)
		content := world.Multiply(scene.Translate(ox, oy))
		clip = clip.Intersect(pixelRect(content.Bounds(sw*scale, sh*scale)))
		for y := clip.Min.Y; y < clip.Max.Y; y++ {
			if err := e.ctx.Err(); err != nil {
				return err
			}
			for x := clip.Min.X; x < clip.Max.X; x++ {
				lx, ly := inv.Point(float64(x)+.5, float64(y)+.5)
				if !inside(lx, n.Bounds.Width) || !inside(ly, n.Bounds.Height) {
					continue
				}
				sx, sy := (lx-ox)/scale, (ly-oy)/scale
				if !inside(sx, sw) || !inside(sy, sh) {
					continue
				}
				layer.Set(x, y, src.At(src.Bounds().Min.X+sampleIndex(sx, src.Bounds().Dx()), src.Bounds().Min.Y+sampleIndex(sy, src.Bounds().Dy())))
			}
		}
	} else {
		for _, c := range n.Children {
			if err := e.paint(layer, c, depth+1); err != nil {
				return err
			}
		}
	}
	var mask image.Image
	if n.Mask != "" {
		mask, err = e.raster.Raster(e.ctx, model.Source{Kind: "image", Path: n.Mask}, e.base)
		if err != nil {
			return fmt.Errorf("layer %s mask: %w", n.ID, err)
		}
		if mask == nil || mask.Bounds().Empty() {
			return fmt.Errorf("layer %s: empty mask", n.ID)
		}
	}
	for y := e.rect.Min.Y; y < e.rect.Max.Y; y++ {
		if err := e.ctx.Err(); err != nil {
			return err
		}
		for x := e.rect.Min.X; x < e.rect.Max.X; x++ {
			factor := n.Opacity
			if mask != nil {
				lx, ly := inv.Point(float64(x)+.5, float64(y)+.5)
				if !image.Pt(x, y).In(footprint) || !inside(lx, n.Bounds.Width) || !inside(ly, n.Bounds.Height) {
					factor = 0
				} else {
					mx := mask.Bounds().Min.X + sampleIndex(lx/n.Bounds.Width*float64(mask.Bounds().Dx()), mask.Bounds().Dx())
					my := mask.Bounds().Min.Y + sampleIndex(ly/n.Bounds.Height*float64(mask.Bounds().Dy()), mask.Bounds().Dy())
					_, _, _, a := mask.At(mx, my).RGBA()
					factor *= float64(a) / 65535
				}
			}
			i := layer.PixOffset(x, y)
			for k := 0; k < 4; k++ {
				layer.Pix[i+k] = uint8(math.Round(float64(layer.Pix[i+k]) * factor))
			}
		}
	}
	draw.Draw(dst, e.rect, layer, image.Point{}, draw.Over)
	return nil
}

func (e *engine) rasterLeaf(n scene.ResolvedNode, world scene.Matrix) (image.Image, error) {
	if sized, ok := e.raster.(render.SceneRasterizer); ok {
		return sized.RasterScene(e.ctx, render.Request{Source: *n.Source, Fit: n.Fit, BaseDir: e.base, Scene: scene.Context{Bounds: n.Bounds, PixelSize: model.Canvas{Width: n.Bounds.Width * math.Hypot(world[0], world[1]), Height: n.Bounds.Height * math.Hypot(world[2], world[3])}}})
	}
	return e.raster.Raster(e.ctx, *n.Source, e.base)
}
func edge(v float64) int {
	half := math.Round(v*2) / 2
	if math.Abs(v-half) <= 1e-12*math.Max(1, math.Abs(v)) {
		v = half
	}
	return int(math.Round(v))
}
func pixelRect(r scene.Rect) image.Rectangle {
	return image.Rect(edge(r.X), edge(r.Y), edge(r.X+r.Width), edge(r.Y+r.Height))
}

// Match rounded clip edges when an inverse-mapped sample lands on a boundary.
func inside(v, extent float64) bool {
	epsilon := 1e-12 * math.Max(1, extent)
	return v >= -epsilon && v <= extent+epsilon
}
func sampleIndex(v float64, extent int) int { return min(max(int(v), 0), extent-1) }
