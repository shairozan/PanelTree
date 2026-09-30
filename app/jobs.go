package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/build"
	"github.com/shairozan/PanelTree/internal/export"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

type Job struct {
	jobs.Job
	GenerationProvenance *GenerationProvenance `json:"generation_provenance,omitempty"`
}
type GenerationProvenance struct {
	BackendIdentity string            `json:"backend_identity"`
	Adapter         string            `json:"adapter"`
	Profile         string            `json:"profile"`
	ProfileHash     string            `json:"profile_hash"`
	Models          map[string]string `json:"models"`
	Seed            uint64            `json:"seed"`
	Output          string            `json:"output"`
	Width           int               `json:"width"`
	Height          int               `json:"height"`
}
type JobDiagnostic = jobs.Diagnostic
type RendererCapability struct {
	OutputKinds []string `json:"output_kinds,omitempty"`
	Profile     string   `json:"profile,omitempty"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	SourceKinds []string `json:"source_kinds"`
	Available   bool     `json:"available"`
}
type AssetRequest struct {
	Generation               *render.Generation
	ProjectFile              string
	ExpectedRevision         model.Revision
	Target                   LayerTarget
	IdempotencyKey, Renderer string
	Width, Height            int
	Fit                      string
}
type JobRequest struct{ ProjectFile, ID string }
type RunJobsRequest struct {
	ProjectFile string
	Workers     int
}
type SelectCandidateRequest struct {
	ProjectFile, JobID string
	ExpectedRevision   model.Revision
}

func (s *Service) Renderers() []RendererCapability {
	caps := []RendererCapability{{Name: "builtin", Version: "static/v1/" + runtime.Version(), SourceKinds: []string{"image", "svg", "text"}, Available: true}, {Name: "comfyui", Version: adapters.ComfyVersion, SourceKinds: []string{"generated"}, OutputKinds: []string{"rgb"}, Available: s.comfy != nil}}
	if s.comfy != nil {
		caps[1].Profile = s.comfy.Profile.Name + "/" + s.comfy.Profile.Revision
	}
	return caps
}

type assetInput struct {
	GenerationRecipe *adapters.ComfyRecipe `json:"generation_recipe,omitempty"`
	Version          string                `json:"version"`
	Target           LayerTarget           `json:"target"`
	Renderer         string                `json:"renderer"`
	RendererVersion  string                `json:"renderer_version"`
	Request          render.Request        `json:"request"`
	Files            map[string][]byte     `json:"files"`
	Width, Height    int
	Fit              string
	Fingerprint      string `json:"fingerprint"`
}

func jobStore(w *workspace.Session) (*jobs.Store, error) {
	path, e := workspace.SafePath(w.Root, ".paneltree/jobs")
	if e != nil {
		return nil, e
	}
	return jobs.Open(path)
}
func (s *Service) openJobs(ctx context.Context, p string) (*jobs.Store, error) {
	var store *jobs.Store
	e := workspace.Open(ctx, p, func(w *workspace.Session) error { var e error; store, e = jobStore(w); return e })
	return store, e
}
func (s *Service) RequestAsset(ctx context.Context, r AssetRequest) (Job, error) {
	if r.Renderer == "" {
		r.Renderer = "builtin"
	}
	if r.Renderer != "builtin" && (r.Renderer != "comfyui" || s.comfy == nil) {
		return Job{}, &JobDiagnostic{Code: "renderer_unavailable", Message: "renderer " + r.Renderer + " is unavailable"}
	}
	if r.Renderer == "builtin" && r.Generation != nil {
		return Job{}, &JobDiagnostic{Code: "invalid_input", Message: "builtin renderer does not accept generation requests"}
	}
	var j jobs.Job
	e := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		if w.Entry != w.Owner {
			return &JobDiagnostic{Code: "invalid_project", Message: "submit against the owning project"}
		}
		store, e := jobStore(w)
		if e != nil {
			return e
		}
		prior, payload, e := store.Lookup(ctx, r.IdempotencyKey)
		if e == nil {
			var original assetInput
			if e = json.Unmarshal(payload, &original); e != nil {
				return e
			}
			if prior.Revision != r.ExpectedRevision || original.Target != r.Target || original.Renderer != r.Renderer || original.Width != r.Width || original.Height != r.Height || original.Fit != r.Fit || hash(original.Request.Generation) != hash(r.Generation) {
				return &JobDiagnostic{Code: "idempotency_conflict", Message: "key already identifies a different request"}
			}
			j = prior
			return nil
		}
		var diagnostic *JobDiagnostic
		if !errors.As(e, &diagnostic) || diagnostic.Code != "not_found" {
			return e
		}
		if r.ExpectedRevision == "" || r.ExpectedRevision != w.Snapshot.Revision {
			return &JobDiagnostic{Code: "revision_conflict", Message: "asset submission requires the current project revision"}
		}
		input, e := s.snapshotAsset(ctx, w, r)
		if e != nil {
			return e
		}
		data, e := json.Marshal(input)
		if e != nil {
			return e
		}
		j, e = store.Submit(ctx, r.IdempotencyKey, w.Snapshot.Revision, data)
		return e
	})
	if e != nil {
		return Job{}, e
	}
	return s.Job(ctx, JobRequest{ProjectFile: r.ProjectFile, ID: j.ID})
}
func (s *Service) Job(ctx context.Context, r JobRequest) (Job, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return Job{}, e
	}
	j, e := store.Get(ctx, r.ID)
	if e != nil {
		return Job{}, e
	}
	return describeJob(ctx, store, j)
}
func (s *Service) Jobs(ctx context.Context, project string) ([]Job, error) {
	store, e := s.openJobs(ctx, project)
	if e != nil {
		return nil, e
	}
	return describeJobs(ctx, store)
}
func (s *Service) CancelJob(ctx context.Context, r JobRequest) (Job, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return Job{}, e
	}
	j, e := store.Cancel(ctx, r.ID)
	if e != nil {
		return Job{}, e
	}
	return describeJob(ctx, store, j)
}
func (s *Service) RunJobs(ctx context.Context, r RunJobsRequest) ([]Job, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return nil, e
	}
	e = store.Run(ctx, r.Workers, func(ctx context.Context, payload json.RawMessage, progress func(int) error) ([]byte, error) {
		var input assetInput
		if e := json.Unmarshal(payload, &input); e != nil {
			return nil, e
		}
		if input.Renderer == "comfyui" {
			return s.runGeneration(ctx, input, progress)
		}
		if input.Version != "asset-job/v1" || input.Renderer != "builtin" || input.RendererVersion != s.Renderers()[0].Version {
			return nil, &JobDiagnostic{Code: "renderer_unavailable", Message: "submitted renderer version is unavailable"}
		}
		dir, e := os.MkdirTemp(store.Root, ".render-")
		if e != nil {
			return nil, e
		}
		defer func() { _ = os.RemoveAll(dir) }()
		for _, rel := range []string{input.Request.Source.Path, input.Request.Source.Font} {
			if rel == "" {
				continue
			}
			if _, e := workspace.SafePath(dir, rel); e != nil {
				return nil, e
			}
			if _, ok := input.Files[rel]; !ok {
				return nil, &JobDiagnostic{Code: "invalid_input", Message: "renderer dependency is absent from frozen inputs"}
			}
		}
		for rel, data := range input.Files {
			path, e := workspace.SafePath(dir, rel)
			if e != nil {
				return nil, e
			}
			if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return nil, e
			}
			if e = os.WriteFile(path, data, 0600); e != nil {
				return nil, e
			}
		}
		if e = progress(20); e != nil {
			return nil, e
		}
		request := input.Request
		request.BaseDir = dir
		im, e := (adapters.Builtin{}).RasterScene(ctx, request)
		if e != nil {
			return nil, e
		}
		if e = progress(80); e != nil {
			return nil, e
		}
		var out bytes.Buffer
		if e = png.Encode(&out, im); e != nil {
			return nil, e
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		return out.Bytes(), nil
	})
	if e != nil {
		return nil, e
	}
	return describeJobs(ctx, store)
}

func describeJob(ctx context.Context, store *jobs.Store, j jobs.Job) (Job, error) {
	out := Job{Job: j}
	_, payload, e := store.Lookup(ctx, j.Key)
	if e != nil {
		return out, e
	}
	var input assetInput
	if e = json.Unmarshal(payload, &input); e != nil {
		return out, e
	}
	if r := input.GenerationRecipe; r != nil {
		out.GenerationProvenance = &GenerationProvenance{BackendIdentity: r.Profile.BackendIdentity, Adapter: r.Adapter, Profile: r.Profile.Name + "/" + r.Profile.Revision, ProfileHash: r.ProfileHash, Models: r.Profile.Models, Seed: r.Generation.Seed, Output: r.Generation.Output, Width: r.Width, Height: r.Height}
	}
	return out, nil
}
func describeJobs(ctx context.Context, store *jobs.Store) ([]Job, error) {
	list, e := store.List(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]Job, 0, len(list))
	for _, j := range list {
		described, e := describeJob(ctx, store, j)
		if e != nil {
			return nil, e
		}
		out = append(out, described)
	}
	return out, nil
}
func (s *Service) SelectCandidate(ctx context.Context, r SelectCandidateRequest) (EditResult, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return EditResult{}, e
	}
	_, data, _, e := store.Result(ctx, r.JobID)
	if e != nil {
		return EditResult{}, e
	}
	var input assetInput
	if e = json.Unmarshal(data, &input); e != nil {
		return EditResult{}, e
	}
	return s.Edit(ctx, EditRequest{ProjectFile: r.ProjectFile, ExpectedRevision: r.ExpectedRevision, Operations: []Operation{{Target: input.Target, Action: "select-candidate", Candidate: r.JobID}}})
}

func (s *Service) snapshotAsset(ctx context.Context, w *workspace.Session, r AssetRequest) (assetInput, error) {
	var input assetInput
	nodes := indexLayers(w.Snapshot)
	n, ok := nodes[targetKey(r.Target)]
	if !ok || n.layer.Source == nil {
		return input, &JobDiagnostic{Code: "invalid_target", Message: "asset requests require a known leaf layer"}
	}
	states, e := readStates(w.State)
	if e != nil {
		return input, e
	}
	if e = checkLocks(nodes, nodes, states, w.Root); e != nil {
		return input, e
	}
	for _, st := range states {
		if st.Pin != "" {
			if _, e = asset.Resolve(w.Root, st.Pin); e != nil {
				return input, e
			}
		}
		if st.Manual != "" {
			if _, e = asset.ReadPNG(st.Manual); e != nil {
				return input, e
			}
		}
	}
	for _, page := range w.Snapshot.Pages {
		if page.Page.ID != r.Target.Page {
			continue
		}
		original, e := adapters.ReadAsset(ctx, "", page.File, 1<<20)
		if e != nil {
			return input, e
		}
		stage, e := export.Prepare(ctx, filepath.Join(w.Root, ".paneltree"), filepath.Dir(page.File), page.Page, original)
		if e != nil {
			return input, e
		}
		defer stage.Abort()
		resolved, e := layout.Resolve(ctx, stage.Page, layout.Options{Width: r.Width, Height: r.Height, Fit: r.Fit, BaseDir: stage.Dir, Measurer: adapters.Builtin{}})
		if e != nil {
			return input, e
		}
		var leaf *scene.ResolvedNode
		var visit func(scene.ResolvedNode, string)
		visit = func(node scene.ResolvedNode, panel string) {
			if node.Kind == "panel" {
				panel = node.ID
			}
			if panel == string(r.Target.Panel) && node.Kind == "layer" && node.ID == string(r.Target.Layer) {
				copy := node
				leaf = &copy
			}
			for _, child := range node.Children {
				visit(child, panel)
			}
		}
		visit(resolved.Tree(), "")
		if leaf == nil || leaf.Source == nil {
			return input, fmt.Errorf("resolved leaf missing")
		}
		request := build.LeafRequest(*leaf, resolved.Output(), "")
		request.Revision = w.Snapshot.Revision
		request.Generation = r.Generation
		files := map[string][]byte{}
		for _, rel := range []string{request.Source.Path, request.Source.Font} {
			if rel == "" {
				continue
			}
			data, e := adapters.ReadAsset(ctx, stage.Dir, rel, 32<<20)
			if e != nil {
				return input, e
			}
			files[rel] = data
		}
		input = assetInput{Version: "asset-job/v1", Target: r.Target, Renderer: r.Renderer, RendererVersion: s.Renderers()[0].Version, Request: request, Files: files, Width: r.Width, Height: r.Height, Fit: r.Fit}
		if r.Renderer == "comfyui" {
			if s.comfy == nil {
				return input, &JobDiagnostic{Code: "renderer_unavailable", Message: "ComfyUI is not configured"}
			}
			recipe, e := s.comfy.Freeze(request)
			if e != nil {
				return input, e
			}
			input.GenerationRecipe = &recipe
			input.RendererVersion = adapters.ComfyVersion
		}
		input.Fingerprint = hash(input)
		return input, nil
	}
	return input, &JobDiagnostic{Code: "invalid_target", Message: "page not found"}
}

func (s *Service) candidate(ctx context.Context, w *workspace.Session, op Operation) (string, error) {
	store, e := jobStore(w)
	if e != nil {
		return "", e
	}
	j, payload, data, e := store.Result(ctx, op.Candidate)
	if e != nil {
		return "", e
	}
	if j.Revision != w.Snapshot.Revision {
		return "", &JobDiagnostic{Code: "stale_candidate", Message: "candidate belongs to an older project revision"}
	}
	var input assetInput
	if e = json.Unmarshal(payload, &input); e != nil {
		return "", e
	}
	if input.Target != op.Target {
		return "", &JobDiagnostic{Code: "invalid_target", Message: "candidate belongs to another layer"}
	}
	current, e := s.snapshotAsset(ctx, w, AssetRequest{Target: op.Target, Renderer: input.Renderer, Width: input.Width, Height: input.Height, Fit: input.Fit, Generation: input.Request.Generation})
	if e != nil {
		return "", e
	}
	if current.Fingerprint != input.Fingerprint {
		return "", &JobDiagnostic{Code: "stale_candidate", Message: "effective inputs changed since submission"}
	}
	if _, e = png.Decode(bytes.NewReader(data)); e != nil {
		return "", e
	}
	return asset.Put(w.Root, data)
}

func (s *Service) runGeneration(ctx context.Context, input assetInput, progress func(int) error) ([]byte, error) {
	if input.Version != "asset-job/v1" || input.RendererVersion != adapters.ComfyVersion || input.GenerationRecipe == nil || s.comfy == nil {
		return nil, &JobDiagnostic{Code: "renderer_unavailable", Message: "submitted ComfyUI renderer is unavailable"}
	}
	// Frozen workflows execute only against their declared backend/model environment.
	if hash(s.comfy.Profile) != hash(input.GenerationRecipe.Profile) {
		return nil, &JobDiagnostic{Code: "renderer_unavailable", Message: "configured generation profile changed since submission"}
	}
	if e := progress(20); e != nil {
		return nil, e
	}
	im, e := s.comfy.Generate(ctx, *input.GenerationRecipe)
	if e != nil {
		return nil, e
	}
	if e = progress(80); e != nil {
		return nil, e
	}
	var out bytes.Buffer
	if e = png.Encode(&out, im); e != nil {
		return nil, e
	}
	return out.Bytes(), ctx.Err()
}
