package app

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/compose"
	"github.com/shairozan/PanelTree/internal/export"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/internal/project"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
)

type BuildRequest struct {
	ProjectFile, PageID, Output, Fit string
	Width, Height                    int
}
type BuildResult struct {
	PageID        string `json:"page_id"`
	Output        string `json:"output"`
	Width, Height int
}

func (s *Service) Build(ctx context.Context, r BuildRequest) (BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}
	if r.Output == "" {
		return BuildResult{}, fmt.Errorf("output path is required")
	}
	path, err := filepath.Abs(r.Output)
	if err != nil {
		return BuildResult{}, err
	}
	if _, err = os.Lstat(path); err == nil {
		return BuildResult{}, fmt.Errorf("output already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return BuildResult{}, err
	}
	snapshot, err := project.Load(r.ProjectFile)
	if err != nil {
		return BuildResult{}, err
	}
	selected := -1
	for i, p := range snapshot.Pages {
		if string(p.Page.ID) == r.PageID || r.PageID == "" && len(snapshot.Pages) == 1 {
			if selected != -1 {
				return BuildResult{}, fmt.Errorf("page ID %q is ambiguous; supply its page YAML directly", r.PageID)
			}
			selected = i
		}
	}
	if selected < 0 {
		return BuildResult{}, fmt.Errorf("select a known page ID with --page (required for multi-page input)")
	}
	p := snapshot.Pages[selected]
	base := filepath.Dir(p.File)
	bg, err := background(p.Page.Background)
	if err != nil {
		return BuildResult{}, err
	}
	resolved, err := layout.Resolve(ctx, p.Page, layout.Options{Width: r.Width, Height: r.Height, Fit: r.Fit, BaseDir: base, Measurer: s.measurer})
	if err != nil {
		return BuildResult{}, err
	}
	raster := s.raster
	if raster == nil {
		raster = adapters.PNG{}
	}
	im, err := compose.Page(ctx, resolved, base, bg, raster)
	if err != nil {
		return BuildResult{}, fmt.Errorf("page %s: %w", p.Page.ID, err)
	}
	if err = export.PNG(ctx, path, im); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{PageID: string(p.Page.ID), Output: path, Width: im.Bounds().Dx(), Height: im.Bounds().Dy()}, nil
}
func background(s string) (color.Color, error) {
	if s == "" {
		return color.Transparent, nil
	}
	if len(s) != 7 && len(s) != 9 || s[0] != '#' {
		return nil, fmt.Errorf("background requires #RRGGBB or #RRGGBBAA")
	}
	n, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("background %q: %w", s, err)
	}
	if len(s) == 7 {
		n = n<<8 | 255
	}
	return color.NRGBA{R: uint8(n >> 24), G: uint8(n >> 16), B: uint8(n >> 8), A: uint8(n)}, nil
}
