// Package mcp exposes shared PanelTree services over local MCP.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type result[T any] struct {
	Data  *T          `json:"data,omitempty"`
	Error *diagnostic `json:"error,omitempty"`
}
type projectInput struct {
	Project string `json:"project_file"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Fit     string `json:"fit,omitempty"`
}
type editInput struct {
	Project    string             `json:"project_file"`
	Revision   model.Revision     `json:"expected_revision"`
	Edits      []app.DocumentEdit `json:"edits,omitempty"`
	Operations []app.Operation    `json:"operations,omitempty"`
}
type assetInput struct {
	Generation *render.Generation `json:"generation,omitempty"`
	Project    string             `json:"project_file"`
	Revision   model.Revision     `json:"expected_revision"`
	Key        string             `json:"key"`
	Target     app.LayerTarget    `json:"target"`
	Renderer   string             `json:"renderer,omitempty"`
	Width      int                `json:"width,omitempty"`
	Height     int                `json:"height,omitempty"`
	Fit        string             `json:"fit,omitempty"`
}
type jobInput struct {
	Project string `json:"project_file"`
	ID      string `json:"id"`
}
type runInput struct {
	Project string `json:"project_file"`
	Workers int    `json:"workers,omitempty"`
}
type selectInput struct {
	Project  string         `json:"project_file"`
	ID       string         `json:"id"`
	Revision model.Revision `json:"expected_revision"`
}
type buildInput struct {
	Project string `json:"project_file"`
	Page    string `json:"page"`
	Output  string `json:"output,omitempty"`
	Bundle  string `json:"bundle,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
	Fit     string `json:"fit,omitempty"`
	NoCache bool   `json:"no_cache,omitempty"`
}
type buildOutput struct {
	Build     app.BuildResult `json:"build"`
	Preview   string          `json:"preview"`
	Artifacts []string        `json:"artifacts"`
}
type projectList struct {
	Projects []string `json:"projects"`
}
type server struct {
	sdk       *protocol.Server
	service   *app.Service
	roots     []string
	gate      chan struct{}
	mu        sync.Mutex
	resources map[string]bool
}

func New(roots []string, options ...app.ServiceOption) (*protocol.Server, error) {
	return NewWithService(roots, app.NewService(options...))
}
func NewWithService(roots []string, service *app.Service) (*protocol.Server, error) {
	s := &server{service: service, gate: make(chan struct{}, 1), resources: map[string]bool{}}
	if len(roots) == 0 {
		return nil, fmt.Errorf("at least one MCP root is required")
	}
	for _, root := range roots {
		p, e := filepath.Abs(root)
		if e != nil {
			return nil, e
		}
		p, e = filepath.EvalSymlinks(p)
		if e != nil {
			return nil, e
		}
		info, e := os.Stat(p)
		if e != nil {
			return nil, e
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("MCP root must be a directory")
		}
		s.roots = append(s.roots, p)
	}
	s.sdk = protocol.NewServer(&protocol.Implementation{Name: "paneltree", Version: "0.1.0"}, nil)
	s.sdk.AddResource(&protocol.Resource{URI: "paneltree://workflow", Name: "Authoring workflow", MIMEType: "text/plain"}, func(context.Context, *protocol.ReadResourceRequest) (*protocol.ReadResourceResult, error) {
		return &protocol.ReadResourceResult{Contents: []*protocol.ResourceContents{{URI: "paneltree://workflow", MIMEType: "text/plain", Text: "Discover or initialize a project. Open it for IDs, documents and revision. Apply document edits and editorial operations with expected_revision. Request a leaf asset with a unique key, run queued jobs, poll status, and explicitly select its candidate. Build PNG or a portable SVG bundle; read returned artifact resource URIs. Reopen after every edit. Roots come only from server configuration; client roots never expand access."}}}, nil
	})
	bind(s, "project_init", "Initialize a new book in an allowed directory", func(ctx context.Context, in struct {
		Directory string `json:"directory"`
	}) (app.InitResult, error) {
		var out app.InitResult
		e := s.guard(ctx, func() error {
			p, e := s.path(in.Directory)
			if e != nil {
				return e
			}
			out, e = s.service.Init(ctx, app.InitRequest{Directory: p})
			if e == nil {
				s.projectResource(out.ProjectFile)
			}
			return e
		})
		return out, e
	})
	bind(s, "project_list", "Discover project.yaml files beneath configured roots", func(ctx context.Context, _ struct{}) (projectList, error) {
		out := projectList{Projects: []string{}}
		e := s.guard(ctx, func() error {
			for _, root := range s.roots {
				e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if e = ctx.Err(); e != nil {
						return e
					}
					if d.IsDir() && (d.Name() == ".paneltree" || d.Name() == ".git") {
						return filepath.SkipDir
					}
					if d.Type()&os.ModeSymlink != 0 {
						return nil
					}
					if !d.IsDir() && d.Name() == "project.yaml" {
						out.Projects = append(out.Projects, p)
					}
					if len(out.Projects) > 1024 {
						return fmt.Errorf("project discovery limit exceeded")
					}
					return nil
				})
				if e != nil {
					return e
				}
			}
			sort.Strings(out.Projects)
			return nil
		})
		return out, e
	})
	for _, name := range []string{"project_open", "project_inspect"} {
		bind(s, name, "Inspect authored documents, stable IDs, revision and resolved scenes", func(ctx context.Context, in projectInput) (*app.Inspection, error) {
			var out *app.Inspection
			e := s.guard(ctx, func() error {
				p, e := s.project(in.Project, nil)
				if e != nil {
					return e
				}
				out, e = s.service.Inspect(ctx, app.InspectRequest{ProjectFile: p, Width: in.Width, Height: in.Height, Fit: in.Fit})
				if e == nil {
					s.projectResource(p)
				}
				return e
			})
			return out, e
		})
	}
	bind(s, "project_validate", "Validate a project through shared policy", func(ctx context.Context, in projectInput) (app.ValidateResult, error) {
		var out app.ValidateResult
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, nil)
			if e != nil {
				return e
			}
			out, e = s.service.Validate(ctx, app.InspectRequest{ProjectFile: p})
			return e
		})
		return out, e
	})
	bind(s, "project_apply_changes", "Apply documents and draft/review/approve/override/clear-selection/lock/unlock operations atomically", func(ctx context.Context, in editInput) (app.EditResult, error) {
		var out app.EditResult
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, in.Edits)
			if e != nil {
				return e
			}
			for i := range in.Operations {
				if a := in.Operations[i].Artifact; a != "" {
					a, e = s.path(a)
					if e != nil {
						return e
					}
					in.Operations[i].Artifact = a
				}
			}
			out, e = s.service.Edit(ctx, app.EditRequest{ProjectFile: p, ExpectedRevision: in.Revision, Edits: in.Edits, Operations: in.Operations})
			return e
		})
		return out, e
	})
	bind(s, "renderer_list", "List available and unavailable renderer capabilities", func(context.Context, struct{}) ([]app.RendererCapability, error) { return s.service.Renderers(), nil })
	bind(s, "asset_request", "Queue a frozen leaf render; completion does not select it", func(ctx context.Context, in assetInput) (app.Job, error) {
		var out app.Job
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, nil)
			if e != nil {
				return e
			}
			out, e = s.service.RequestAsset(ctx, app.AssetRequest{ProjectFile: p, ExpectedRevision: in.Revision, IdempotencyKey: in.Key, Target: in.Target, Renderer: in.Renderer, Width: in.Width, Height: in.Height, Fit: in.Fit, Generation: in.Generation})
			return e
		})
		return out, e
	})
	bind(s, "asset_select", "Select a successful candidate at its original current revision", func(ctx context.Context, in selectInput) (app.EditResult, error) {
		var out app.EditResult
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, nil)
			if e != nil {
				return e
			}
			out, e = s.service.SelectCandidate(ctx, app.SelectCandidateRequest{ProjectFile: p, JobID: in.ID, ExpectedRevision: in.Revision})
			return e
		})
		return out, e
	})
	bind(s, "jobs_list", "List durable job states and diagnostics", func(ctx context.Context, in projectInput) ([]app.Job, error) {
		var out []app.Job
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, nil)
			if e != nil {
				return e
			}
			out, e = s.service.Jobs(ctx, p)
			return e
		})
		return out, e
	})
	for _, name := range []string{"jobs_status", "jobs_cancel"} {
		bind(s, name, "Read or cancel a durable job", func(ctx context.Context, in jobInput) (app.Job, error) {
			var out app.Job
			e := s.guard(ctx, func() error {
				p, e := s.project(in.Project, nil)
				if e != nil {
					return e
				}
				r := app.JobRequest{ProjectFile: p, ID: in.ID}
				if name == "jobs_cancel" {
					out, e = s.service.CancelJob(ctx, r)
				} else {
					out, e = s.service.Job(ctx, r)
				}
				return e
			})
			return out, e
		})
	}
	bind(s, "jobs_run", "Drain queued jobs with 1–8 workers; cancellation propagates", func(ctx context.Context, in runInput) ([]app.Job, error) {
		var p string
		e := s.guard(ctx, func() error { var e error; p, e = s.project(in.Project, nil); return e })
		if e != nil {
			return nil, e
		}
		if in.Workers == 0 {
			in.Workers = 2
		}
		return s.service.RunJobs(ctx, app.RunJobsRequest{ProjectFile: p, Workers: in.Workers})
	})
	bind(s, "build", "Build a new PNG or portable editable SVG bundle with preview resource URI", func(ctx context.Context, in buildInput) (buildOutput, error) {
		var out buildOutput
		e := s.guard(ctx, func() error {
			p, e := s.project(in.Project, nil)
			if e != nil {
				return e
			}
			if in.Output != "" {
				in.Output, e = s.path(in.Output)
				if e != nil {
					return e
				}
			}
			if in.Bundle != "" {
				in.Bundle, e = s.path(in.Bundle)
				if e != nil {
					return e
				}
				if e = s.tree(in.Bundle); e != nil {
					return e
				}
			}
			out.Build, e = s.service.Build(ctx, app.BuildRequest{ProjectFile: p, PageID: in.Page, Output: in.Output, BundleRoot: in.Bundle, Width: in.Width, Height: in.Height, Fit: in.Fit, NoCache: in.NoCache})
			if e != nil {
				return e
			}
			out.Preview, e = s.artifact(out.Build.Output)
			if e != nil {
				return e
			}
			out.Artifacts = []string{out.Preview}
			if out.Build.Bundle != "" {
				e = filepath.WalkDir(out.Build.Bundle, func(path string, d os.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if !d.IsDir() && path != out.Build.Output {
						uri, e := s.artifact(path)
						if e != nil {
							return e
						}
						out.Artifacts = append(out.Artifacts, uri)
					}
					return nil
				})
			}
			return e
		})
		return out, e
	})
	return s.sdk, nil
}
func bind[I, O any](s *server, name, description string, fn func(context.Context, I) (O, error)) {
	protocol.AddTool(s.sdk, &protocol.Tool{Name: name, Description: description, InputSchema: schema[I](), OutputSchema: schema[result[O]]()}, func(ctx context.Context, _ *protocol.CallToolRequest, in I) (*protocol.CallToolResult, result[O], error) {
		value, e := fn(ctx, in)
		out := result[O]{Data: &value}
		r := &protocol.CallToolResult{}
		if e != nil {
			code := "operation_failed"
			var d *app.JobDiagnostic
			if errors.As(e, &d) {
				code = d.Code
			}
			if errors.Is(e, context.Canceled) {
				code = "cancelled"
			}
			out = result[O]{Error: &diagnostic{Code: code, Message: e.Error()}}
			r.IsError = true
		}
		// Supply structured errors too; the SDK otherwise omits typed output on errors.
		if r.IsError {
			r.StructuredContent = out
			b, _ := json.Marshal(out)
			r.Content = []protocol.Content{&protocol.TextContent{Text: string(b)}}
		}
		return r, out, nil
	})
}
func (s *server) guard(ctx context.Context, fn func() error) error {
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
		if e := ctx.Err(); e != nil {
			return e
		}
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
