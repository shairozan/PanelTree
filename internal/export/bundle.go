package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"go.yaml.in/yaml/v3"
	"image"
	"os"
	"path/filepath"
)

const BundleVersion = "paneltree-bundle/v0.4.0"

type Provenance struct {
	Role     string `json:"role"`
	Original string `json:"original"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
}
type Stage struct {
	Dir, root  string
	Page       model.Page
	Provenance []Provenance
	bytes      int64
	published  bool
}
type Composition struct {
	Version    string         `json:"version"`
	BuildID    string         `json:"build_id"`
	Page       model.Page     `json:"page"`
	Scene      scene.Resolved `json:"scene"`
	Provenance []Provenance   `json:"provenance"`
}

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// Prepare freezes local dependencies before layout/rendering. Generated paths
// are derived only from hashes; original authoring files are never rewritten.
func Prepare(ctx context.Context, root, base string, page model.Page, original []byte) (*Stage, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, ".paneltree-")
	if err != nil {
		return nil, err
	}
	s := &Stage{Dir: dir, root: root}
	success := false
	defer func() {
		if !success {
			s.Abort()
		}
	}()
	if err = os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		return nil, err
	}
	if err = os.Mkdir(filepath.Join(dir, "originals"), 0755); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(encoded, &s.Page); err != nil {
		return nil, err
	}
	if _, err = s.store("authoring", "page.yaml", "originals/", ".yaml", original); err != nil {
		return nil, err
	}
	freeze := func(role, path, ext string, limit int64) (string, error) {
		b, e := adapters.ReadAsset(ctx, base, path, limit)
		if e != nil {
			return "", fmt.Errorf("%s %s: %w", role, path, e)
		}
		return s.store(role, path, "assets/", ext, b)
	}
	var walk func([]model.Layer) error
	walk = func(layers []model.Layer) error {
		for i := range layers {
			l := &layers[i]
			if err := ctx.Err(); err != nil {
				return err
			}
			if l.Source != nil {
				src := l.Source
				switch src.Kind {
				case "image", "svg":
					ext, limit := ".png", int64(32<<20)
					if src.Kind == "svg" {
						ext = ".svg"
						limit = 1 << 20
					}
					src.Path, err = freeze(src.Kind, src.Path, ext, limit)
					if err != nil {
						return err
					}
				case "text":
					old := src.Font
					ext := ".ttf"
					if filepath.Ext(old) == ".otf" {
						ext = ".otf"
					}
					src.Font, err = freeze("font", old, ext, 8<<20)
					if err != nil {
						return err
					}
					license, e := adapters.ReadAsset(ctx, base, old+".LICENSE", 1<<20)
					if e == nil {
						if _, e = s.store("font-license", old+".LICENSE", "assets/", ".txt", license); e != nil {
							return e
						}
						// Keep the discovery convention intact after relocation/re-export.
						if e = os.WriteFile(filepath.Join(s.Dir, filepath.FromSlash(src.Font+".LICENSE")), license, 0644); e != nil {
							return e
						}
					} else if !os.IsNotExist(e) {
						return e
					}
				default:
					return fmt.Errorf("unsupported source %s", src.Kind)
				}
			}
			if l.Mask != "" {
				l.Mask, err = freeze("mask", l.Mask, ".png", 32<<20)
				if err != nil {
					return err
				}
			}
			if err = walk(l.Children); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range s.Page.Panels {
		if err = walk(s.Page.Panels[i].Layers); err != nil {
			return nil, err
		}
	}
	success = true
	return s, nil
}
func (s *Stage) store(role, original, prefix, ext string, b []byte) (string, error) {
	s.bytes += int64(len(b))
	if s.bytes > 128<<20 {
		return "", fmt.Errorf("bundle dependencies exceed 128 MiB")
	}
	hash := digest(b)
	rel := prefix + hash + ext
	if err := os.WriteFile(filepath.Join(s.Dir, filepath.FromSlash(rel)), b, 0644); err != nil {
		return "", err
	}
	s.Provenance = append(s.Provenance, Provenance{Role: role, Original: original, Path: rel, SHA256: hash})
	return rel, nil
}

// Abort only removes this operation's private staging directory.
func (s *Stage) Abort() {
	if !s.published {
		_ = os.RemoveAll(s.Dir)
	}
}
func (s *Stage) Complete(ctx context.Context, resolved scene.Resolved, im image.Image) (string, string, error) {
	if err := PNG(ctx, filepath.Join(s.Dir, "page.png"), im); err != nil {
		return "", "", err
	}
	if err := s.editableSVG(ctx, resolved); err != nil {
		return "", "", err
	}
	doc := model.Document{Schema: model.Schema, Page: &s.Page}
	b, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(s.Dir, "page.yaml"), b, 0644); err != nil {
		return "", "", err
	}
	manifest := Composition{Version: BundleVersion, Page: s.Page, Scene: resolved, Provenance: s.Provenance}
	identity, err := json.Marshal(manifest)
	if err != nil {
		return "", "", err
	}
	manifest.BuildID = digest(identity)
	b, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(s.Dir, "composition.json"), b, 0644); err != nil {
		return "", "", err
	}
	if err = ctx.Err(); err != nil {
		return "", "", err
	}
	// Reserve a fresh parent even for an identical build. Rename publishes the
	// complete payload atomically into this exclusively owned directory.
	parent, err := os.MkdirTemp(s.root, manifest.BuildID+"-")
	if err != nil {
		return "", "", err
	}
	dest := filepath.Join(parent, "bundle")
	if err = os.Rename(s.Dir, dest); err != nil {
		_ = os.Remove(parent)
		return "", "", err
	}
	s.published = true
	return dest, manifest.BuildID, nil
}
