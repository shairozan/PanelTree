package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type ReferenceRequest struct {
	Generation  *render.Generation `json:"generation,omitempty"`
	Width       int                `json:"width,omitempty"`
	Height      int                `json:"height,omitempty"`
	Key         string             `json:"key,omitempty"`
	JobID       string             `json:"job_id,omitempty"`
	ProjectFile string             `json:"project_file"`
	Set         string             `json:"set"`
	Action      string             `json:"action"`
	Revision    string             `json:"revision,omitempty"`
	Slot        string             `json:"slot,omitempty"`
	Candidate   string             `json:"candidate,omitempty"`
	Path        string             `json:"path,omitempty"`
	License     string             `json:"license,omitempty"`
	Attribution string             `json:"attribution,omitempty"`
	Version     string             `json:"version,omitempty"`
	Reapprove   bool               `json:"reapprove,omitempty"`
}
type ReferenceSet struct {
	Jobs       map[string]string               `json:"jobs,omitempty"`
	ID         string                          `json:"id"`
	Sequence   int                             `json:"sequence"`
	Revision   string                          `json:"revision"`
	Accepted   map[string]string               `json:"accepted"`
	Candidates map[string]ReferenceCandidate   `json:"candidates"`
	Published  map[string]ReferencePublication `json:"published"`
}
type ReferenceCandidate struct {
	ImportAttempt   int                   `json:"import_attempt,omitempty"`
	JobID           string                `json:"job_id,omitempty"`
	Recipe          *adapters.ComfyRecipe `json:"recipe,omitempty"`
	Rejected        bool                  `json:"rejected,omitempty"`
	ApprovedAgainst map[string]string     `json:"approved_against"`
	Slot            string                `json:"slot"`
	Image           string                `json:"image"`
	Parents         map[string]string     `json:"parents"`
	License         string                `json:"license"`
	Attribution     string                `json:"attribution"`
}
type ReferencePublication struct {
	Candidates map[string]ReferenceCandidate `json:"candidates"`
	Packing    string                        `json:"packing"`
	Accepted   map[string]string             `json:"accepted"`
	Cards      map[string]string             `json:"cards"`
}

var referenceID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type ImageProvenance struct {
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
}
type ImageCapability struct {
	MaxDimension  int                     `json:"max_dimension"`
	Bindings      []adapters.ImageBinding `json:"bindings"`
	Format        string                  `json:"format"`
	MaxPixels     int                     `json:"max_pixels"`
	MaxTotalBytes int                     `json:"max_total_bytes"`
	Transport     string                  `json:"transport"`
	Packing       []string                `json:"packing"`
}

func imageLimitations() []string {
	return []string{"reference-images: actual frozen pixels; exact ordered roles/count required by profile", "identity-consistency: not guaranteed; img2img can copy reference layout or resist pose changes", "pose and asymmetric details require explicit visual review; no automatic acceptance", "uploads: content-addressed PNGs retained in backend input/paneltree; manual cleanup after jobs finish"}
}

func publishedImages(root string, selection model.ReferenceSelection) ([]render.ImageInput, error) {
	set, e := loadReference(root, selection.Set)
	if e != nil {
		return nil, e
	}
	p, ok := set.Published[selection.Version]
	if !ok {
		return nil, fmt.Errorf("unpublished reference version")
	}
	if len(selection.Directions) < 1 || len(selection.Directions) > 4 {
		return nil, fmt.Errorf("select 1–4 reference directions")
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, direction := range selection.Directions {
		id, ok := p.Cards[direction]
		if !ok || seen[direction] {
			return nil, fmt.Errorf("invalid or duplicate direction")
		}
		seen[direction] = true
		ids = append(ids, id)
	}
	roles := []string{}
	switch selection.Packing {
	case "cards-row/v1":
		if len(ids) > 1 {
			id, e := packReferences(root, ids)
			if e != nil {
				return nil, e
			}
			ids = []string{id}
		}
		roles = []string{"reference"}
	case "directional-cards/v1":
		for _, direction := range selection.Directions {
			roles = append(roles, "card/"+direction)
		}
	default:
		return nil, fmt.Errorf("unsupported reference packing")
	}
	out := []render.ImageInput{}
	for i, id := range ids {
		path, e := asset.Resolve(root, id)
		if e != nil {
			return nil, e
		}
		data, e := asset.ReadPNG(path)
		if e != nil {
			return nil, e
		}
		out = append(out, render.ImageInput{Role: roles[i], SHA256: id, PNG: data})
	}
	return out, nil
}

var referenceSlots = []string{"body/front", "head/front", "body/left", "body/right", "body/rear", "head/left", "head/right", "head/rear"}

func referenceParents(slot string) ([]string, error) {
	switch slot {
	case "body/front":
		return nil, nil
	case "head/front", "body/left", "body/right", "body/rear":
		return []string{"body/front"}, nil
	case "head/left":
		return []string{"head/front", "body/left"}, nil
	case "head/right":
		return []string{"head/front", "body/right"}, nil
	case "head/rear":
		return []string{"head/front", "body/rear"}, nil
	default:
		return nil, fmt.Errorf("unknown reference slot %q", slot)
	}
}
func currentParents(set ReferenceSet, slot string) (map[string]string, error) {
	names, e := referenceParents(slot)
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	for _, name := range names {
		id := set.Accepted[name]
		if id == "" {
			return nil, fmt.Errorf("accept %s first", name)
		}
		out[name] = id
	}
	return out, nil
}
func referenceDir(root, id string) (string, error) {
	if !referenceID.MatchString(id) {
		return "", fmt.Errorf("invalid reference set ID")
	}
	return workspace.SafePath(root, ".paneltree/references/"+id)
}
func loadReference(root, id string) (ReferenceSet, error) {
	var set ReferenceSet
	dir, e := referenceDir(root, id)
	if e != nil {
		return set, e
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return set, e
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && regexp.MustCompile(`^[0-9]{8}\.json$`).MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return set, os.ErrNotExist
	}
	sort.Strings(names)
	path, e := workspace.SafePath(root, filepath.Join(".paneltree/references", id, names[len(names)-1]))
	if e != nil {
		return set, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return set, e
	}
	if e = json.Unmarshal(data, &set); e != nil {
		return set, e
	}
	rev := set.Revision
	set.Revision = ""
	if hash(set) != rev {
		return set, fmt.Errorf("corrupt reference manifest")
	}
	set.Revision = rev
	return set, nil
}
func saveReference(root string, set *ReferenceSet) error {
	dir, e := referenceDir(root, set.ID)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	set.Sequence++
	set.Revision = ""
	set.Revision = hash(*set)
	data, e := json.MarshalIndent(set, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".pending-")
	if e != nil {
		return e
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(dir, fmt.Sprintf("%08d.json", set.Sequence)))
}

// Reference serializes reference design changes with workspace writes, but keeps
// their approval state independent of editorial selections and project revision.
func (s *Service) Reference(ctx context.Context, r ReferenceRequest) (ReferenceSet, error) {
	var set ReferenceSet
	e := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		if w.Entry != w.Owner {
			return fmt.Errorf("use the owning project")
		}
		var e error
		set, e = loadReference(w.Root, r.Set)
		if r.Action == "create" {
			if e == nil {
				return fmt.Errorf("reference set exists")
			}
			if !os.IsNotExist(e) {
				return e
			}
			set = ReferenceSet{ID: r.Set, Accepted: map[string]string{}, Candidates: map[string]ReferenceCandidate{}, Published: map[string]ReferencePublication{}}
			return saveReference(w.Root, &set)
		}
		if e != nil {
			return e
		}
		if r.Action == "inspect" {
			return nil
		}
		if r.Action == "request" {
			return s.requestReference(ctx, w, r, &set)
		}
		if r.Revision == "" || r.Revision != set.Revision {
			return fmt.Errorf("reference revision conflict")
		}
		switch r.Action {
		case "collect":
			store, e := jobStore(w)
			if e != nil {
				return e
			}
			_, payload, data, e := store.Result(ctx, r.JobID)
			if e != nil {
				return e
			}
			var input assetInput
			if e = json.Unmarshal(payload, &input); e != nil {
				return e
			}
			if input.Reference == nil || input.Reference.Request.Set != r.Set {
				return fmt.Errorf("job belongs to another reference set")
			}
			frozen := input.Reference
			id, e := asset.Put(w.Root, data)
			if e != nil {
				return e
			}
			candidate := ReferenceCandidate{Slot: frozen.Request.Slot, Image: id, Parents: frozen.Parents, License: frozen.Request.License, Attribution: frozen.Request.Attribution, JobID: r.JobID, Recipe: input.GenerationRecipe}
			if id := hash(candidate); set.Candidates[id].Image == "" {
				set.Candidates[id] = candidate
			}
		case "import":
			parents, e := currentParents(set, r.Slot)
			if e != nil {
				return e
			}
			if r.License == "" || r.Attribution == "" {
				return fmt.Errorf("import requires license and attribution")
			}
			path, e := workspace.SafePath(w.Root, r.Path)
			if e != nil {
				return e
			}
			data, e := asset.ReadPNG(path)
			if e != nil {
				return e
			}
			id, e := asset.Put(w.Root, data)
			if e != nil {
				return e
			}
			candidate := ReferenceCandidate{Slot: r.Slot, Image: id, Parents: parents, License: r.License, Attribution: r.Attribution}
			if set.Candidates[hash(candidate)].Rejected {
				candidate.ImportAttempt = set.Sequence + 1
			}
			if id := hash(candidate); set.Candidates[id].Image == "" {
				set.Candidates[id] = candidate
			}
		case "accept":
			candidate, ok := set.Candidates[r.Candidate]
			if candidate.Rejected {
				return fmt.Errorf("rejected candidate requires a new import or generation")
			}
			if !ok || candidate.Slot != r.Slot {
				return fmt.Errorf("candidate does not belong to slot")
			}
			parents, e := currentParents(set, r.Slot)
			if e != nil {
				return e
			}
			if hash(parents) != hash(candidate.Parents) && !r.Reapprove {
				return fmt.Errorf("stale candidate: explicitly reapprove against current parents or regenerate")
			}
			if _, e = asset.Resolve(w.Root, candidate.Image); e != nil {
				return e
			}
			candidate.ApprovedAgainst = parents
			set.Candidates[r.Candidate] = candidate
			set.Accepted[r.Slot] = r.Candidate
			// Topological slot order propagates invalidation through head/body descendants.
			for _, slot := range referenceSlots {
				id := set.Accepted[slot]
				if id == "" {
					continue
				}
				c := set.Candidates[id]
				p, e := currentParents(set, slot)
				if e != nil || hash(p) != hash(c.ApprovedAgainst) {
					delete(set.Accepted, slot)
				}
			}
		case "reject":
			c, ok := set.Candidates[r.Candidate]
			if !ok || c.Slot != r.Slot {
				return fmt.Errorf("candidate does not belong to slot")
			}
			if set.Accepted[r.Slot] == r.Candidate {
				return fmt.Errorf("accepted reference cannot be rejected; accept a replacement")
			}
			// Originals and lineage remain available for inspection, even after rejection.
			c.Rejected = true
			set.Candidates[r.Candidate] = c
		case "publish":
			if !referenceID.MatchString(r.Version) {
				return fmt.Errorf("invalid publication version")
			}
			if _, ok := set.Published[r.Version]; ok {
				return fmt.Errorf("published version is immutable")
			}
			p := ReferencePublication{Accepted: map[string]string{}, Cards: map[string]string{}, Packing: "directional-card/v1", Candidates: map[string]ReferenceCandidate{}}
			for _, slot := range referenceSlots {
				id := set.Accepted[slot]
				if id == "" {
					return fmt.Errorf("unapproved slot %s", slot)
				}
				c := set.Candidates[id]
				parents, e := currentParents(set, slot)
				if e != nil || hash(parents) != hash(c.ApprovedAgainst) {
					return fmt.Errorf("stale approval %s", slot)
				}
				if _, e = asset.Resolve(w.Root, c.Image); e != nil {
					return e
				}
				p.Accepted[slot] = id
				p.Candidates[id] = c
			}
			for _, direction := range []string{"front", "left", "right", "rear"} {
				ids := []string{set.Candidates[set.Accepted["body/"+direction]].Image, set.Candidates[set.Accepted["head/"+direction]].Image}
				card, e := packReferences(w.Root, ids)
				if e != nil {
					return e
				}
				p.Cards[direction] = card
			}
			set.Published[r.Version] = p
		default:
			return fmt.Errorf("unknown reference action %q", r.Action)
		}
		return saveReference(w.Root, &set)
	})
	return set, e
}

// Equal 512px cells preserve aspect ratio and original orientation. Originals
// remain independently addressable; cards are derived delivery artifacts.
type referenceJob struct {
	Request ReferenceRequest  `json:"request"`
	Parents map[string]string `json:"parents"`
	Packing string            `json:"packing"`
}

func (s *Service) requestReference(ctx context.Context, w *workspace.Session, r ReferenceRequest, set *ReferenceSet) error {
	if s.comfy == nil {
		return fmt.Errorf("configure a ComfyUI profile")
	}
	store, e := jobStore(w)
	if e != nil {
		return e
	}
	prior, payload, e := store.Lookup(ctx, r.Key)
	if e == nil {
		var input assetInput
		if e = json.Unmarshal(payload, &input); e != nil {
			return e
		}
		if input.Reference == nil || hash(input.Reference.Request) != hash(r) {
			return fmt.Errorf("idempotency conflict")
		}
		if set.Jobs[r.Key] == prior.ID {
			return nil
		}
		if set.Jobs == nil {
			set.Jobs = map[string]string{}
		}
		set.Jobs[r.Key] = prior.ID
		return saveReference(w.Root, set)
	}
	var diagnostic *JobDiagnostic
	if !errors.As(e, &diagnostic) || diagnostic.Code != "not_found" {
		return e
	}
	if r.Revision == "" || r.Revision != set.Revision {
		return fmt.Errorf("reference revision conflict")
	}
	if r.Generation == nil || r.License == "" || r.Attribution == "" {
		return fmt.Errorf("generation requires prompt, license and attribution")
	}
	parents, e := currentParents(*set, r.Slot)
	if e != nil {
		return e
	}
	names, _ := referenceParents(r.Slot)
	ids := []string{}
	for _, name := range names {
		ids = append(ids, set.Candidates[parents[name]].Image)
	}
	req := render.Request{Generation: r.Generation, Scene: scene.Context{PixelSize: model.Canvas{Width: float64(r.Width), Height: float64(r.Height)}}}
	g := *r.Generation
	g.Prompt = referencePose(r.Slot) + "\n" + g.Prompt
	req.Generation = &g
	packing := "original/v1"
	if len(ids) > 0 {
		id := ids[0]
		if len(ids) > 1 {
			id, e = packReferences(w.Root, ids)
			if e != nil {
				return e
			}
			packing = "anchors-row/v1"
		}
		path, e := asset.Resolve(w.Root, id)
		if e != nil {
			return e
		}
		data, e := asset.ReadPNG(path)
		if e != nil {
			return e
		}
		req.ImageInputs = []render.ImageInput{{Role: "reference", SHA256: id, PNG: data}}
	}
	recipe, e := s.comfy.Freeze(req)
	if e != nil {
		return e
	}
	input := assetInput{Version: "asset-job/v1", Renderer: "comfyui", RendererVersion: adapters.ComfyVersion, Request: req, GenerationRecipe: &recipe, Reference: &referenceJob{Request: r, Parents: parents, Packing: packing}}
	data, e := json.Marshal(input)
	if e != nil {
		return e
	}
	j, e := store.Submit(ctx, r.Key, w.Snapshot.Revision, data)
	if e != nil {
		return e
	}
	if set.Jobs == nil {
		set.Jobs = map[string]string{}
	}
	set.Jobs[r.Key] = j.ID
	return saveReference(w.Root, set)
}
func referencePose(slot string) string {
	switch slot {
	case "body/front":
		return "One person, full body visible head to feet, standing upright facing forward, arms resting at sides, neutral expression, plain background, even lighting."
	case "head/front":
		return "Front-facing head detail of the reference person, same identity, hair and asymmetric features, neutral expression, plain background."
	default:
		names := map[string]string{"body/left": "full body, anatomical left profile (left side visible)", "body/right": "full body, anatomical right profile (right side visible)", "body/rear": "full body viewed from behind", "head/left": "head detail, anatomical left profile (left side visible)", "head/right": "head detail, anatomical right profile (right side visible)", "head/rear": "head detail viewed from behind"}
		return "Same reference person, " + names[slot] + ", preserve identity, costume and asymmetric features; plain background, even lighting. Propose unseen details for review."
	}
}

func packReferences(root string, ids []string) (string, error) {
	if len(ids) < 1 || len(ids) > 8 {
		return "", fmt.Errorf("packing requires 1–8 images")
	}
	out := image.NewRGBA(image.Rect(0, 0, 512*len(ids), 512))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, id := range ids {
		path, e := asset.Resolve(root, id)
		if e != nil {
			return "", e
		}
		data, e := asset.ReadPNG(path)
		if e != nil {
			return "", e
		}
		im, e := png.Decode(bytes.NewReader(data))
		if e != nil {
			return "", e
		}
		b := im.Bounds()
		width, height := 512, 512
		if b.Dx() > b.Dy() {
			height = 512 * b.Dy() / b.Dx()
		} else {
			width = 512 * b.Dx() / b.Dy()
		}
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		ox, oy := i*512+(512-width)/2, (512-height)/2
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				out.Set(ox+x, oy+y, im.At(b.Min.X+x*b.Dx()/width, b.Min.Y+y*b.Dy()/height))
			}
		}
	}
	var b bytes.Buffer
	if e := png.Encode(&b, out); e != nil {
		return "", e
	}
	return asset.Put(root, b.Bytes())
}
