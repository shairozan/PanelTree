// Package export publishes complete files without replacing existing work.
package export

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

type writer struct {
	ctx context.Context
	w   io.Writer
}

func (w writer) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

// PNG encodes beside the destination, then links the finished file exclusively.
// Filesystems without hard-link support return an error, never an unsafe fallback.
func PNG(ctx context.Context, path string, im image.Image) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".paneltree-*.png")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	if err = png.Encode(writer{ctx, f}, im); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); err != nil {
		return fmt.Errorf("publish new output (existing files are protected): %w", err)
	}
	return nil
}
