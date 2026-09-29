// Package app exposes shared operations for CLI, MCP and future web callers.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"path/filepath"
)

type Snapshot = project.Snapshot
type InitRequest struct{ Directory string }
type InitResult struct {
	ProjectFile string `json:"project_file"`
}
type InspectRequest struct {
	ProjectFile   string
	Width, Height int
	Fit           string
}
type ResolvedPage struct {
	File  string         `json:"file"`
	Scene scene.Resolved `json:"scene"`
}
type Inspection struct {
	*Snapshot
	Layers map[string]LayerStatus `json:"layers,omitempty"`
	Scenes []ResolvedPage         `json:"scenes"`
}
type ValidateResult struct {
	Valid     bool `json:"valid"`
	PageCount int  `json:"page_count"`
}
type Service struct {
	measurer scene.Measurer
	raster   render.Rasterizer
}
type ServiceOption func(*Service)

func WithMeasurer(m scene.Measurer) ServiceOption      { return func(s *Service) { s.measurer = m } }
func WithRasterizer(r render.Rasterizer) ServiceOption { return func(s *Service) { s.raster = r } }

func NewService(options ...ServiceOption) *Service {
	s := &Service{measurer: adapters.Builtin{}, raster: adapters.Builtin{}}
	for _, option := range options {
		option(s)
	}
	return s
}
func (s *Service) Init(ctx context.Context, r InitRequest) (InitResult, error) {
	if err := ctx.Err(); err != nil {
		return InitResult{}, err
	}
	path, err := project.Init(r.Directory)
	return InitResult{ProjectFile: path}, err
}
func (s *Service) Inspect(ctx context.Context, r InspectRequest) (*Inspection, error) {
	var result *Inspection
	err := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		data, e := json.Marshal(w.Snapshot)
		if e != nil {
			return e
		}
		var original Snapshot
		if e = json.Unmarshal(data, &original); e != nil {
			return e
		}
		states, e := selectedSnapshot(w)
		if e != nil {
			return e
		}
		result, e = s.inspect(ctx, r, w.Snapshot)
		if result != nil {
			result.Snapshot = &original
			result.Layers = states
		}
		return e
	})
	return result, err
}
func (s *Service) inspect(ctx context.Context, r InspectRequest, snapshot *project.Snapshot) (*Inspection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &Inspection{Snapshot: snapshot}
	for _, page := range snapshot.Pages {
		resolved, err := layout.Resolve(ctx, page.Page, layout.Options{Width: r.Width, Height: r.Height, Fit: r.Fit, BaseDir: filepath.Dir(page.File), Measurer: s.measurer})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", page.File, err)
		}
		result.Scenes = append(result.Scenes, ResolvedPage{File: page.File, Scene: resolved})
	}
	return result, nil
}
func (s *Service) Validate(ctx context.Context, r InspectRequest) (ValidateResult, error) {
	if err := ctx.Err(); err != nil {
		return ValidateResult{}, err
	}
	var result ValidateResult
	err := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		if _, e := selectedSnapshot(w); e != nil {
			return e
		}
		result = ValidateResult{Valid: true, PageCount: len(w.Snapshot.Pages)}
		return nil
	})
	return result, err
}
