package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/render"
)

func (s *Service) resolveGeneration(renderer string, g *render.Generation) (string, *render.Generation, error) {
	if g == nil {
		if renderer == "" {
			renderer = "builtin"
		}
		return renderer, g, nil
	}
	copy := *g
	copy.StyleReferences = append([]string(nil), g.StyleReferences...)
	g = &copy
	if g.Profile == "" && (renderer == "" || renderer == "ideogram") {
		g.Profile = s.generation.DefaultProfile
	}
	if g.Profile != "" {
		p, ok := s.generation.Profiles[g.Profile]
		if !ok {
			return "", nil, fmt.Errorf("unknown generation profile %q", g.Profile)
		}
		if renderer != "" && renderer != p.Renderer {
			return "", nil, fmt.Errorf("renderer conflicts with generation profile")
		}
		renderer = p.Renderer
	}
	if renderer == "" {
		return "", nil, fmt.Errorf("generation requires a configured profile or renderer")
	}
	if renderer == "ideogram" && g.Profile == "" {
		return "", nil, fmt.Errorf("ideogram requires a generation profile")
	}
	return renderer, g, nil
}
func readGenerationImage(root, path, role string) (render.ImageInput, error) {
	resolved, e := workspace.SafePath(root, path)
	if e != nil {
		return render.ImageInput{}, e
	}
	data, e := asset.ReadPNG(resolved)
	if e != nil {
		return render.ImageInput{}, e
	}
	return render.ImageInput{Role: role, SHA256: asset.Digest(data), PNG: data}, nil
}
func (s *Service) freezeIdeogram(root string, input *assetInput) error {
	req := input.Request
	if req.Generation == nil {
		return fmt.Errorf("missing generation settings")
	}
	p, ok := s.generation.Profiles[req.Generation.Profile]
	if !ok {
		return fmt.Errorf("unknown generation profile")
	}

	if req.Generation.CharacterReference != "" {
		if len(req.ImageInputs) > 0 {
			return fmt.Errorf("explicit character image conflicts with selected reference")
		}
		im, e := readGenerationImage(root, req.Generation.CharacterReference, "character")
		if e != nil {
			return e
		}
		req.ImageInputs = append(req.ImageInputs, im)
	} else if req.Character != nil && req.Character.Use.ReferenceSet != nil {
		return fmt.Errorf("ideogram requires an explicit character-reference image; published cards are not silently packed")
	}
	for _, path := range req.Generation.StyleReferences {
		im, e := readGenerationImage(root, path, "style")
		if e != nil {
			return e
		}
		req.ImageInputs = append(req.ImageInputs, im)
	}
	recipe, e := adapters.FreezeIdeogram(req.Generation.Profile, p, req)
	if e != nil {
		return e
	}
	input.IdeogramRecipe = &recipe
	input.RendererVersion = adapters.IdeogramVersion
	// Freeze consumes reference bytes; paths remain in the original request for freshness checks.
	return nil
}
func (s *Service) runIdeogram(ctx context.Context, input assetInput) ([]byte, error) {
	if input.Version != "asset-job/v1" || input.RendererVersion != adapters.IdeogramVersion || input.IdeogramRecipe == nil || s.ideogram == nil {
		return nil, fmt.Errorf("submitted Ideogram renderer unavailable")
	}
	var state adapters.IdeogramState
	if data := jobs.Execution(ctx); len(data) > 0 {
		if e := json.Unmarshal(data, &state); e != nil {
			return nil, e
		}
	}
	return s.ideogram.Run(ctx, *input.IdeogramRecipe, state, func(v adapters.IdeogramState) error { return jobs.Checkpoint(ctx, v, v.GenerationID != "") })
}

func (s *Service) ResumeJob(ctx context.Context, r JobRequest) (Job, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return Job{}, e
	}
	j, e := store.Resume(ctx, r.ID)
	if e != nil {
		return Job{}, e
	}
	return describeJob(ctx, store, j)
}
func (s *Service) EstimateJob(ctx context.Context, r JobRequest) (json.RawMessage, error) {
	store, e := s.openJobs(ctx, r.ProjectFile)
	if e != nil {
		return nil, e
	}
	j, e := store.Get(ctx, r.ID)
	if e != nil {
		return nil, e
	}
	_, data, e := store.Lookup(ctx, j.Key)
	if e != nil {
		return nil, e
	}
	var input assetInput
	if e = json.Unmarshal(data, &input); e != nil {
		return nil, e
	}
	if input.IdeogramRecipe == nil || s.ideogram == nil {
		return nil, fmt.Errorf("only Ideogram jobs support price estimation")
	}
	return s.ideogram.Estimate(ctx, *input.IdeogramRecipe)
}
