package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const ComfyVersion = "comfyui/v1"

type ComfyNode struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
}
type ComfyBinding struct {
	Node  string `json:"node"`
	Input string `json:"input"`
}
type ComfyProfile struct {
	ImageBindings   []ImageBinding          `json:"image_bindings,omitempty"`
	Version         string                  `json:"version"`
	Name            string                  `json:"name"`
	Revision        string                  `json:"revision"`
	BackendIdentity string                  `json:"backend_identity"`
	Models          map[string]string       `json:"models"`
	OutputNode      string                  `json:"output_node"`
	Workflow        map[string]ComfyNode    `json:"workflow"`
	Bindings        map[string]ComfyBinding `json:"bindings"`
}
type ComfyRecipe struct {
	ImageInputs []render.ImageInput      `json:"image_inputs,omitempty"`
	Character   *model.ResolvedCharacter `json:"character,omitempty"`
	Adapter     string                   `json:"adapter"`
	Profile     ComfyProfile             `json:"profile"`
	ProfileHash string                   `json:"profile_hash"`
	Generation  render.Generation        `json:"generation"`
	Width       int                      `json:"width"`
	Height      int                      `json:"height"`
	Workflow    map[string]ComfyNode     `json:"workflow"`
}
type ComfyUI struct {
	URL                   string
	Profile               ComfyProfile
	Timeout, PollInterval time.Duration
}

func LoadComfyProfile(path string) (ComfyProfile, error) {
	var p ComfyProfile
	f, e := os.Open(path)
	if e != nil {
		return p, e
	}
	defer func() { _ = f.Close() }()
	d := json.NewDecoder(io.LimitReader(f, 2<<20))
	d.DisallowUnknownFields()
	d.UseNumber()
	if e = d.Decode(&p); e != nil {
		return p, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return p, fmt.Errorf("profile requires one JSON document")
	}
	return p, p.validate()
}
func (p ComfyProfile) validate() error {
	if err := p.validateImageBindings(); err != nil {
		return err
	}
	if p.Version != "comfy-profile/v1" || p.Name == "" || p.Revision == "" || p.BackendIdentity == "" || len(p.Models) == 0 {
		return fmt.Errorf("profile requires version, name, revision, backend_identity and model identities")
	}
	for name, id := range p.Models {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(id) == "" {
			return fmt.Errorf("empty model identity")
		}
	}
	if _, ok := p.Workflow[p.OutputNode]; !ok {
		return fmt.Errorf("profile output node is missing")
	}
	for _, key := range []string{"prompt", "seed", "width", "height"} {
		if _, ok := p.Bindings[key]; !ok {
			return fmt.Errorf("missing %s binding", key)
		}
	}
	used := map[ComfyBinding]bool{}
	for key, b := range p.Bindings {
		switch key {
		case "prompt", "negative_prompt", "seed", "width", "height":
		default:
			return fmt.Errorf("unknown semantic binding %s", key)
		}
		if used[b] {
			return fmt.Errorf("duplicate input binding")
		}
		used[b] = true
		node, ok := p.Workflow[b.Node]
		if !ok {
			return fmt.Errorf("binding node %s missing", b.Node)
		}
		if _, ok = node.Inputs[b.Input]; !ok {
			return fmt.Errorf("binding input %s missing", b.Input)
		}
	}
	for _, n := range p.Workflow {
		if n.ClassType == "" || n.Inputs == nil {
			return fmt.Errorf("workflow requires class_type and inputs")
		}
	}
	return nil
}

// Preserve large integer seeds and workflow constants across durable JSON snapshots.
func (n *ComfyNode) UnmarshalJSON(data []byte) error {
	type plain ComfyNode
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	return d.Decode((*plain)(n))
}
func (c *ComfyUI) Freeze(r render.Request) (ComfyRecipe, error) {
	var out ComfyRecipe
	if e := c.Profile.validate(); e != nil {
		return out, e
	}
	if r.Generation == nil || strings.TrimSpace(r.Generation.Prompt) == "" {
		return out, fmt.Errorf("generation requires a prompt")
	}
	g := *r.Generation
	if g.CharacterReference != "" || len(g.StyleReferences) > 0 {
		return out, fmt.Errorf("ComfyUI profile does not support character-image/style-image options; use its explicit image bindings")
	}
	if len(g.Prompt) > 16384 || len(g.NegativePrompt) > 16384 {
		return out, fmt.Errorf("generation prompt too long")
	}
	if g.Output != "" && g.Output != "rgb" {
		return out, fmt.Errorf("ComfyUI supports RGB only; isolated RGBA requires a separate matting adapter")
	}
	g.Output = "rgb"
	if g.NegativePrompt != "" {
		if _, ok := c.Profile.Bindings["negative_prompt"]; !ok {
			return out, fmt.Errorf("profile does not support negative_prompt")
		}
	}
	w, h := math.Ceil(r.Scene.PixelSize.Width/8)*8, math.Ceil(r.Scene.PixelSize.Height/8)*8
	if math.IsNaN(w) || math.IsNaN(h) || w < 8 || h < 8 || w > 8192 || h > 8192 || w*h > 4<<20 {
		return out, fmt.Errorf("generation dimensions exceed limits")
	}
	data, e := json.Marshal(c.Profile)
	if e != nil {
		return out, e
	}
	out = ComfyRecipe{Adapter: ComfyVersion, ProfileHash: fmt.Sprintf("%x", sha256.Sum256(data)), Generation: g, Width: int(w), Height: int(h)}
	if e = json.Unmarshal(data, &out.Profile); e != nil {
		return out, e
	}
	var detached ComfyProfile
	if e = json.Unmarshal(data, &detached); e != nil {
		return out, e
	}
	out.Workflow = detached.Workflow
	if e = freezeImages(r.ImageInputs, &out); e != nil {
		return out, e
	}
	if r.Character != nil {
		data, err := json.Marshal(r.Character)
		if err != nil {
			return out, err
		}
		if err = json.Unmarshal(data, &out.Character); err != nil {
			return out, err
		}
		g.Prompt += characterPrompt(out.Character)
		if len(g.Prompt) > 16384 {
			return out, fmt.Errorf("resolved character prompt too long")
		}
	}
	values := map[string]any{"prompt": g.Prompt, "negative_prompt": g.NegativePrompt, "seed": g.Seed, "width": out.Width, "height": out.Height}
	for name, b := range out.Profile.Bindings {
		out.Workflow[b.Node].Inputs[b.Input] = values[name]
	}
	return out, nil
}
func (c *ComfyUI) Generate(ctx context.Context, r ComfyRecipe) (result image.Image, err error) {
	if e := ValidateComfyURL(c.URL); e != nil {
		return nil, e
	}
	// Reject altered recipes and unsupported outputs before contacting the backend.
	check := &ComfyUI{Profile: r.Profile}
	expected, e := check.Freeze(render.Request{ImageInputs: r.ImageInputs, Character: r.Character, Generation: &r.Generation, Scene: scene.Context{PixelSize: model.Canvas{Width: float64(r.Width), Height: float64(r.Height)}}})
	if e != nil {
		return nil, e
	}
	a, _ := json.Marshal(expected)
	b, _ := json.Marshal(r)
	if !bytes.Equal(a, b) {
		return nil, fmt.Errorf("invalid frozen generation recipe")
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, stop := context.WithTimeout(ctx, timeout)
	defer stop()
	var nodes map[string]json.RawMessage
	if e = c.getJSON(ctx, "/object_info", &nodes); e != nil {
		return nil, e
	}
	for _, n := range r.Workflow {
		if _, ok := nodes[n.ClassType]; !ok {
			return nil, fmt.Errorf("backend lacks workflow node %s", n.ClassType)
		}
	}
	body, e := json.Marshal(map[string]any{"prompt": r.Workflow, "extra_data": map[string]any{"paneltree": map[string]any{"profile_hash": r.ProfileHash, "backend_identity": r.Profile.BackendIdentity, "models": r.Profile.Models, "seed": r.Generation.Seed}}})
	if e != nil {
		return nil, e
	}
	for _, input := range r.ImageInputs {
		if e = c.uploadImage(ctx, input); e != nil {
			return nil, e
		}
	}
	// POST /prompt has no guaranteed idempotency. Never retry an ambiguous response.
	data, e := c.http(ctx, http.MethodPost, "/prompt", body, 2<<20)
	if e != nil {
		return nil, fmt.Errorf("submission failed (not retried; backend acceptance may be unknown): %w", e)
	}
	var submitted struct {
		ID string `json:"prompt_id"`
	}
	if e = json.Unmarshal(data, &submitted); e != nil || submitted.ID == "" {
		return nil, fmt.Errorf("ambiguous submission response; not retried")
	}
	id := url.PathEscape(submitted.ID)
	defer func() {
		if err == nil {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// This endpoint is scoped atomically; never fall back to global /interrupt.
		if _, e := c.http(cleanup, http.MethodPost, "/api/jobs/"+id+"/cancel", []byte(`{}`), 2<<20); e != nil {
			err = errors.Join(err, fmt.Errorf("backend cancellation could not be confirmed: %w", e))
		}
	}()
	for {
		var history map[string]struct {
			Status struct {
				Completed bool   `json:"completed"`
				Status    string `json:"status_str"`
			} `json:"status"`
			Outputs map[string]struct {
				Images []struct {
					Filename  string `json:"filename"`
					Subfolder string `json:"subfolder"`
					Type      string `json:"type"`
				} `json:"images"`
			} `json:"outputs"`
		}
		if e = c.getJSON(ctx, "/history/"+id, &history); e != nil {
			return nil, e
		}
		h, ok := history[submitted.ID]
		if ok && (h.Status.Status == "error" || h.Status.Status == "cancelled") {
			return nil, fmt.Errorf("ComfyUI execution %s", h.Status.Status)
		}
		if ok && h.Status.Completed {
			images := h.Outputs[r.Profile.OutputNode].Images
			if h.Status.Status != "success" || len(images) != 1 || images[0].Filename == "" {
				return nil, fmt.Errorf("workflow must produce exactly one successful output image")
			}
			ref := images[0]
			q := url.Values{"filename": {ref.Filename}, "subfolder": {ref.Subfolder}, "type": {ref.Type}}
			data, e := c.get(ctx, "/view?"+q.Encode(), 32<<20)
			if e != nil {
				return nil, e
			}
			cfg, e := png.DecodeConfig(bytes.NewReader(data))
			if e != nil {
				return nil, fmt.Errorf("backend requires PNG output: %w", e)
			}
			if cfg.Width != r.Width || cfg.Height != r.Height {
				return nil, fmt.Errorf("backend output dimensions %dx%d differ from requested %dx%d", cfg.Width, cfg.Height, r.Width, r.Height)
			}
			im, e := png.Decode(bytes.NewReader(data))
			if e != nil {
				return nil, e
			}
			// Flatten any unexpected alpha against white; never label RGB as a cutout.
			rgb := image.NewNRGBA(im.Bounds())
			for y := 0; y < cfg.Height; y++ {
				if e = ctx.Err(); e != nil {
					return nil, e
				}
				for x := 0; x < cfg.Width; x++ {
					rr, gg, bb, aa := im.At(x, y).RGBA()
					white := uint32(65535) - aa
					rgb.SetNRGBA(x, y, color.NRGBA{R: uint8((rr + white) >> 8), G: uint8((gg + white) >> 8), B: uint8((bb + white) >> 8), A: 255})
				}
			}
			return rgb, nil
		}
		if e = c.pause(ctx); e != nil {
			return nil, e
		}
	}
}

func ValidateComfyURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("ComfyUI URL requires an HTTP(S) endpoint without credentials, query or fragment")
	}
	return nil
}

type comfyHTTPError struct {
	status int
	detail string
}

func (e *comfyHTTPError) Error() string {
	return fmt.Sprintf("ComfyUI HTTP status %d: %s", e.status, e.detail)
}
func (c *ComfyUI) http(ctx context.Context, method, path string, body []byte, limit int64) ([]byte, error) {
	return c.httpContent(ctx, method, path, body, limit, "application/json")
}
func (c *ComfyUI) httpContent(ctx context.Context, method, path string, body []byte, limit int64, contentType string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, bytes.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", contentType)
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &comfyHTTPError{status: resp.StatusCode, detail: strings.TrimSpace(string(data))}
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("backend response exceeds size limit")
	}
	return data, nil
}
func (c *ComfyUI) pause(ctx context.Context) error {
	interval := c.PollInterval
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (c *ComfyUI) get(ctx context.Context, path string, limit int64) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		data, e := c.http(ctx, http.MethodGet, path, nil, limit)
		if e == nil {
			return data, nil
		}
		var status *comfyHTTPError
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		retry := errors.As(e, &status) && (status.status == 429 || status.status >= 500)
		var network *url.Error
		retry = retry || errors.As(e, &network)
		if !retry || attempt == 2 {
			return nil, e
		}
		if e = c.pause(ctx); e != nil {
			return nil, e
		}
	}
}
func (c *ComfyUI) getJSON(ctx context.Context, path string, dst any) error {
	data, e := c.get(ctx, path, 8<<20)
	if e != nil {
		return e
	}
	return json.Unmarshal(data, dst)
}

// ComfyUI implements the leaf raster contract; measurement remains local.
func (c *ComfyUI) Raster(context.Context, model.Source, string) (image.Image, error) {
	return nil, fmt.Errorf("ComfyUI requires a scene and semantic generation request")
}
func (c *ComfyUI) RasterScene(ctx context.Context, r render.Request) (image.Image, error) {
	recipe, e := c.Freeze(r)
	if e != nil {
		return nil, e
	}
	return c.Generate(ctx, recipe)
}
func (c *ComfyUI) CacheRecipe(_ context.Context, r render.Request) (any, error) { return c.Freeze(r) }

var _ render.CacheRasterizer = (*ComfyUI)(nil)
