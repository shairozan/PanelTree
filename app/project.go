// Package app exposes shared operations for CLI, MCP and future web callers.
package app

import (
	"context"
	"github.com/shairozan/PanelTree/internal/project"
)

type Snapshot = project.Snapshot
type InitRequest struct{ Directory string }
type InitResult struct {
	ProjectFile string `json:"project_file"`
}
type InspectRequest struct{ ProjectFile string }
type ValidateResult struct {
	Valid     bool `json:"valid"`
	PageCount int  `json:"page_count"`
}
type Service struct{}

func NewService() *Service { return &Service{} }
func (s *Service) Init(ctx context.Context, r InitRequest) (InitResult, error) {
	if err := ctx.Err(); err != nil {
		return InitResult{}, err
	}
	path, err := project.Init(r.Directory)
	return InitResult{ProjectFile: path}, err
}
func (s *Service) Inspect(ctx context.Context, r InspectRequest) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return project.Load(r.ProjectFile)
}
func (s *Service) Validate(ctx context.Context, r InspectRequest) (ValidateResult, error) {
	snapshot, err := s.Inspect(ctx, r)
	if err != nil {
		return ValidateResult{}, err
	}
	return ValidateResult{Valid: true, PageCount: len(snapshot.Pages)}, nil
}
