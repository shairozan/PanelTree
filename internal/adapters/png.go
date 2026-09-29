// Package adapters provides leaf renderer implementations.
package adapters

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

type PNG struct{}

func (PNG) Raster(ctx context.Context, s model.Source, base string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Kind != "image" {
		return nil, fmt.Errorf("source kind %q: PNG renderer supports image only", s.Kind)
	}
	path := s.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	im, err := readPNG(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("PNG %s: %w", path, err)
	}
	return im, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func readPNG(ctx context.Context, path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return nil, fmt.Errorf("requires regular PNG at most 32 MiB")
	}
	cfg, err := png.DecodeConfig(contextReader{ctx, io.LimitReader(f, 32<<20)})
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
		return nil, fmt.Errorf("dimensions exceed 8192 per axis or 4194304 pixels")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return png.Decode(contextReader{ctx, io.LimitReader(f, 32<<20)})
}
