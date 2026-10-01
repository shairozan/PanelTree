package app

import (
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/internal/workspace"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) openWorkspace(ctx context.Context, handle string, fn func(*workspace.Session) error) error {
	if strings.HasPrefix(handle, "pg:") {
		if s.storage == nil {
			return fmt.Errorf("PostgreSQL storage is not configured")
		}
		return s.storage.Open(ctx, strings.TrimPrefix(handle, "pg:"), func(w *workspace.Session) error { w.Handle = handle; return fn(w) })
	}
	return workspace.Open(ctx, handle, fn)
}

func (s *Service) ConfigureStorage(ctx context.Context, c storage.Config) error {
	if c.DSNEnv == "" && c.BlobRoot == "" {
		return nil
	}
	if c.DSNEnv == "" {
		c.DSNEnv = "PANELTREE_DATABASE_URL"
	}
	p, e := storage.Connect(ctx, os.Getenv(c.DSNEnv), c.BlobRoot)
	if e != nil {
		return e
	}
	s.storage = p
	return nil
}
func (s *Service) HasStorage() bool { return s.storage != nil }

type StorageRequest struct {
	Action      string `json:"action"`
	ProjectFile string `json:"project_file,omitempty"`
	ID          string `json:"id,omitempty"`
	Path        string `json:"path,omitempty"`
}
type StorageResult struct {
	Projects []string `json:"projects,omitempty"`
	Project  string   `json:"project,omitempty"`
}

func (s *Service) Storage(ctx context.Context, r StorageRequest) (StorageResult, error) {
	var out StorageResult
	if s.storage == nil {
		return out, fmt.Errorf("PostgreSQL storage is not configured")
	}
	var e error
	switch r.Action {
	case "migrate":
		e = s.storage.Migrate(ctx)
	case "list":
		out.Projects, e = s.storage.Projects(ctx)
	case "import":
		e = s.storage.Import(ctx, r.ID, r.ProjectFile)
		out.Project = "pg:" + r.ID
	case "export":
		if !strings.HasPrefix(r.ProjectFile, "pg:") {
			return out, fmt.Errorf("export requires pg:project-id")
		}
		e = s.storage.Export(ctx, strings.TrimPrefix(r.ProjectFile, "pg:"), r.Path)
	default:
		e = fmt.Errorf("unknown storage action")
	}
	return out, e
}
func (s *Service) initDatabase(ctx context.Context, handle string) (InitResult, error) {
	if s.storage == nil {
		return InitResult{}, fmt.Errorf("PostgreSQL storage is not configured")
	}
	dir, e := os.MkdirTemp("", "paneltree-init-")
	if e != nil {
		return InitResult{}, e
	}
	defer func() { _ = os.RemoveAll(dir) }()
	entry, e := project.Init(filepath.Join(dir, "source"))
	if e != nil {
		return InitResult{}, e
	}
	if e = s.storage.Import(ctx, strings.TrimPrefix(handle, "pg:"), entry); e != nil {
		return InitResult{}, e
	}
	return InitResult{ProjectFile: handle}, nil
}
