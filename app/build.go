package app

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/build"
	"github.com/shairozan/PanelTree/internal/cache"
	"github.com/shairozan/PanelTree/internal/compose"
	"github.com/shairozan/PanelTree/internal/export"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
)

type BuildRequest struct {
	CacheDir                         string
	NoCache                          bool
	BundleRoot                       string
	ProjectFile, PageID, Output, Fit string
	Width, Height                    int
}
type BuildResult struct {
	Layers                      map[string]LayerStatus `json:"layers,omitempty"`
	Revision                    model.Revision         `json:"revision"`
	LeafRenders, Recompositions int
	Cache                       *build.Report `json:"cache,omitempty"`
	Bundle                      string        `json:"bundle,omitempty"`
	BuildID                     string        `json:"build_id,omitempty"`
	PageID                      string        `json:"page_id"`
	Output                      string        `json:"output"`
	Width, Height               int
}

func (s *Service) Build(ctx context.Context, r BuildRequest) (BuildResult, error) {
	var result BuildResult
	err := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		states, e := selectedSnapshot(w)
		if e != nil {
			return e
		}
		result, e = s.build(ctx, r, w.Snapshot)
		result.Layers = states
		result.Revision = w.Snapshot.Revision
		return e
	})
	return result, err
}
func (s *Service) build(ctx context.Context, r BuildRequest, snapshot *project.Snapshot) (BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}
	if (r.Output == "") == (r.BundleRoot == "") {
		return BuildResult{}, fmt.Errorf("choose exactly one output path or bundle root")
	}
	path, err := filepath.Abs(r.Output)
	if err != nil {
		return BuildResult{}, err
	}
	if r.BundleRoot == "" {
		if _, err = os.Lstat(path); err == nil {
			return BuildResult{}, fmt.Errorf("output already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return BuildResult{}, err
		}
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
	// The built-in service freezes dependencies before planning. Injected adapters
	// remain uncached until their service supplies an equivalent snapshot boundary.
	_, builtinRaster := s.raster.(adapters.Builtin)
	_, builtinMeasure := s.measurer.(adapters.Builtin)
	useCache := !r.NoCache && builtinRaster && builtinMeasure
	var store *cache.Store
	if useCache {
		root := r.CacheDir
		if root == "" {
			root = filepath.Join(filepath.Dir(r.ProjectFile), ".paneltree", "cache")
		}
		store, err = cache.Open(root)
		if err != nil {
			return BuildResult{}, err
		}
	}
	var stage *export.Stage
	if r.BundleRoot != "" || useCache {
		if _, ok := s.raster.(adapters.Builtin); !ok {
			return BuildResult{}, fmt.Errorf("portable bundle requires built-in rasterizer")
		}
		if _, ok := s.measurer.(adapters.Builtin); !ok {
			return BuildResult{}, fmt.Errorf("portable bundle requires built-in measurer")
		}
		original, e := adapters.ReadAsset(ctx, "", p.File, 1<<20)
		if e != nil {
			return BuildResult{}, e
		}
		root := r.BundleRoot
		if root == "" {
			root = filepath.Join(store.Root, "staging")
		}
		stage, err = export.Prepare(ctx, root, base, p.Page, original)
		if err != nil {
			return BuildResult{}, err
		}
		defer stage.Abort()
		p.Page = stage.Page
		base = stage.Dir
	}
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
		raster = adapters.Builtin{}
	}
	var session *build.Session
	var memo compose.Memo
	if useCache {
		session, err = build.New(ctx, store, raster.(render.CacheRasterizer), base, resolved)
		if err != nil {
			return BuildResult{}, err
		}
		raster = session
		memo = session
	}
	im, err := compose.PageWithMemo(ctx, resolved, base, bg, raster, memo)
	if err != nil {
		return BuildResult{}, fmt.Errorf("page %s: %w", p.Page.ID, err)
	}
	result := BuildResult{PageID: string(p.Page.ID), Output: path, Width: im.Bounds().Dx(), Height: im.Bounds().Dy()}
	var encoded []byte
	if session != nil {
		encoded, result.BuildID, err = session.Export(ctx, im, p.Page.Background)
		if err != nil {
			return BuildResult{}, err
		}
		result.Cache = &session.Report
		result.LeafRenders = session.Report.LeafRenders
		result.Recompositions = session.Report.Recompositions
	}
	if r.BundleRoot != "" {
		bundle, id, e := stage.CompleteCached(ctx, resolved, im, encoded, result.BuildID)
		if e != nil {
			return BuildResult{}, e
		}
		result.Output = filepath.Join(bundle, "page.png")
		result.Bundle = bundle
		result.BuildID = id
		return result, nil
	}
	if encoded != nil {
		err = export.PNGBytes(ctx, path, encoded)
	} else {
		err = export.PNG(ctx, path, im)
	}
	if err != nil {
		return BuildResult{}, err
	}
	return result, nil
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
