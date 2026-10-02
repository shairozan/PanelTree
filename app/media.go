package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/workspace"
	"image/png"
	"os"
	"path/filepath"
)

type ArtworkRequest struct {
	ProjectFile          string
	PNG                  []byte
	License, Attribution string
}
type Artwork struct {
	Path        string `json:"path"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
}

func validateArtwork(data []byte) error {
	if len(data) == 0 || len(data) > 32<<20 {
		return fmt.Errorf("PNG must be at most 32 MiB")
	}
	cfg, e := png.DecodeConfig(bytes.NewReader(data))
	if e != nil {
		return fmt.Errorf("PNG required: %w", e)
	}
	if cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
		return fmt.Errorf("image exceeds pixel limits")
	}
	_, e = png.Decode(bytes.NewReader(data))
	return e
}
func (s *Service) ImportArtwork(ctx context.Context, r ArtworkRequest) (Artwork, error) {
	var out Artwork
	if e := validateArtwork(r.PNG); e != nil {
		return out, e
	}
	if len(r.License) > 4096 || len(r.Attribution) > 4096 {
		return out, fmt.Errorf("provenance exceeds limit")
	}
	e := s.openWorkspace(ctx, r.ProjectFile, func(w *workspace.Session) error {
		id, e := asset.Put(w.Root, r.PNG)
		if e != nil {
			return e
		}
		out = Artwork{Path: ".paneltree/assets/" + id + ".png", License: r.License, Attribution: r.Attribution}
		b, e := json.Marshal(out)
		if e != nil {
			return e
		}
		dir, e := workspace.SafePath(w.Root, ".paneltree/imports")
		if e != nil {
			return e
		}
		if e = os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		path, e := workspace.SafePath(w.Root, ".paneltree/imports/"+asset.Digest(b)+".json")
		if e != nil {
			return e
		}
		return os.WriteFile(path, b, 0600)
	})
	return out, e
}

// Media exposes only bounded verified PNGs, never arbitrary files or job recipes.
func (s *Service) Media(ctx context.Context, project, path, job string) ([]byte, error) {
	if job != "" {
		store, e := s.openJobs(ctx, project)
		if e != nil {
			return nil, e
		}
		_, _, data, e := store.Result(ctx, job)
		if e != nil {
			return nil, e
		}
		if e = validateArtwork(data); e != nil {
			return nil, e
		}
		return data, nil
	}
	var out []byte
	e := s.openWorkspace(ctx, project, func(w *workspace.Session) error {
		p, e := workspace.SafePath(w.Root, path)
		if e != nil {
			return e
		}
		out, e = asset.ReadPNG(p)
		return e
	})
	return out, e
}

// Artworks lists imported originals and user-supplied provenance, without selecting them.
func (s *Service) Artworks(ctx context.Context, project string) ([]Artwork, error) {
	out := []Artwork{}
	e := s.openWorkspace(ctx, project, func(w *workspace.Session) error {
		dir, e := workspace.SafePath(w.Root, ".paneltree/imports")
		if e != nil {
			return e
		}
		entries, e := os.ReadDir(dir)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if len(entries) > 10000 {
			return fmt.Errorf("artwork list exceeds limit")
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			p, e := workspace.SafePath(w.Root, filepath.Join(".paneltree/imports", entry.Name()))
			if e != nil {
				return e
			}
			st, e := os.Stat(p)
			if e != nil {
				return e
			}
			if st.Size() > 16384 {
				return fmt.Errorf("artwork metadata exceeds limit")
			}
			data, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			var a Artwork
			if e = json.Unmarshal(data, &a); e != nil {
				return e
			}
			out = append(out, a)
		}
		return nil
	})
	return out, e
}

func (s *Service) JobSource(ctx context.Context, project, id string) ([]byte, error) {
	store, e := s.openJobs(ctx, project)
	if e != nil {
		return nil, e
	}
	j, e := store.Get(ctx, id)
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
	images := input.Request.ImageInputs
	if input.IdeogramRecipe != nil {
		images = input.IdeogramRecipe.Images
	}
	for _, im := range images {
		if im.Role == "character" || im.Role == "reference" {
			if e = validateArtwork(im.PNG); e != nil {
				return nil, e
			}
			return im.PNG, nil
		}
	}
	return nil, fmt.Errorf("job has no source image")
}
