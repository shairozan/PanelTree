package adapters

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/cache"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

type Builtin struct{}

func (Builtin) CacheRecipe(ctx context.Context, r render.Request) (any, error) {
	s := r.Source
	for _, p := range []*string{&s.Path, &s.Font} {
		if *p != "" {
			b, err := ReadAsset(ctx, r.BaseDir, *p, 32<<20)
			if err != nil {
				return nil, err
			}
			*p = cache.Hash(b)
		}
	}
	// PNG decoding is independent of placement and target resolution.
	if s.Kind == "image" {
		r.Scene = scene.Context{}
		r.Fit = ""
	} else {
		r.Scene.Bounds.X = 0
		r.Scene.Bounds.Y = 0
	}
	return struct {
		Version string
		Source  model.Source
		Fit     string
		Scene   scene.Context
	}{"builtin/" + s.Kind + "/v1/x-image-v0.46.0/" + runtime.Version(), s, r.Fit, r.Scene}, nil
}

func (Builtin) Raster(ctx context.Context, s model.Source, base string) (image.Image, error) {
	if s.Kind == "image" {
		return (PNG{}).Raster(ctx, s, base)
	}
	return (Builtin{}).RasterScene(ctx, render.Request{Source: s, BaseDir: base})
}
func (Builtin) RasterScene(ctx context.Context, r render.Request) (image.Image, error) {
	switch r.Source.Kind {
	case "image":
		return (PNG{}).Raster(ctx, r.Source, r.BaseDir)
	case "svg":
		return rasterSVG(ctx, r)
	case "text":
		return rasterText(ctx, r)
	default:
		return nil, fmt.Errorf("unsupported raster kind %q", r.Source.Kind)
	}
}
func (Builtin) Measure(ctx context.Context, r scene.MeasureRequest) (scene.Metrics, error) {
	if r.Source.Kind != "text" {
		return scene.Metrics{}, ctx.Err()
	}
	p, err := PlanText(ctx, r.Source, r.BaseDir, r.Constraints.Max.Width, r.Constraints.Max.Height)
	if err != nil {
		return scene.Metrics{}, err
	}
	return scene.Metrics{Minimum: model.Canvas{Width: p.MinimumWidth, Height: p.LineHeight + 2}, Preferred: model.Canvas{Width: p.Width, Height: p.Height}}, nil
}

// ReadAsset snapshots bounded regular-file content; no network or external resolution.
func ReadAsset(ctx context.Context, base, path string, limit int64) ([]byte, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%s exceeds regular-file limit %d", path, limit)
	}
	b, err := io.ReadAll(contextReader{ctx, io.LimitReader(f, limit+1)})
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, limit)
	}
	return b, nil
}
func ParseColor(s string) (color.NRGBA, error) {
	if s == "" {
		return color.NRGBA{A: 255}, nil
	}
	if s == "none" {
		return color.NRGBA{}, nil
	}
	if (len(s) != 7 && len(s) != 9) || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("color %q requires #RRGGBB or #RRGGBBAA", s)
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, err
	}
	if len(s) == 7 {
		n = n<<8 | 255
	}
	return color.NRGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
}
func rasterSize(w, h float64) (int, int, error) {
	if math.IsNaN(w) || math.IsNaN(h) || math.IsInf(w, 0) || math.IsInf(h, 0) || w <= 0 || h <= 0 || w > 8192 || h > 8192 || math.Ceil(w)*math.Ceil(h) > 4<<20 {
		return 0, 0, fmt.Errorf("leaf raster exceeds 8192 per axis or 4194304 pixels")
	}
	return int(math.Ceil(w)), int(math.Ceil(h)), nil
}
