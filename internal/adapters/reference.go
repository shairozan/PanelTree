package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	assets "github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/render"
	"image/png"
	"mime/multipart"
	"net/http"
)

// ImageBinding declares an exact ordered role and a LoadImage input. Workflows
// must connect the image to conditioning; ordinary prompt bindings cannot do so.
type ImageBinding struct {
	Role string `json:"role"`
	Node string `json:"node"`
}

func (p ComfyProfile) validateImageBindings() error {
	if len(p.ImageBindings) > 8 {
		return fmt.Errorf("at most eight reference images supported")
	}
	used := map[string]bool{}
	for _, b := range p.ImageBindings {
		n, ok := p.Workflow[b.Node]
		if b.Role == "" || used[b.Node] || !ok || n.ClassType != "LoadImage" {
			return fmt.Errorf("image binding requires a unique LoadImage node and role")
		}
		if _, ok = n.Inputs["image"]; !ok {
			return fmt.Errorf("LoadImage input missing")
		}
		for _, v := range p.Bindings {
			if v.Node == b.Node {
				return fmt.Errorf("image and text bindings overlap")
			}
		}
		used[b.Node] = true
		if !p.imageReachesOutput(b.Node) {
			return fmt.Errorf("reference image is disconnected from output")
		}
	}
	return nil
}

func (p ComfyProfile) imageReachesOutput(target string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, value := range p.Workflow[id].Inputs {
			link, ok := value.([]any)
			if !ok || len(link) != 2 {
				continue
			}
			parent, ok := link[0].(string)
			if parent == target {
				if fmt.Sprint(link[1]) == "0" {
					return true
				}
				continue
			}
			if ok && visit(parent) {
				return true
			}
		}
		return false
	}
	return visit(p.OutputNode)
}

func freezeImages(inputs []render.ImageInput, out *ComfyRecipe) error {
	if len(inputs) != len(out.Profile.ImageBindings) {
		return fmt.Errorf("profile requires exactly %d image inputs; got %d", len(out.Profile.ImageBindings), len(inputs))
	}
	total := 0
	for i, v := range inputs {
		total += len(v.PNG)
		if total > 32<<20 || v.Role != out.Profile.ImageBindings[i].Role || v.SHA256 != assets.Digest(v.PNG) {
			return fmt.Errorf("invalid reference role, hash or size")
		}
		cfg, e := png.DecodeConfig(bytes.NewReader(v.PNG))
		if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
			return fmt.Errorf("reference requires PNG within 8192 dimensions and four megapixels")
		}
		if _, e = png.Decode(bytes.NewReader(v.PNG)); e != nil {
			return e
		}
		v.PNG = bytes.Clone(v.PNG)
		out.ImageInputs = append(out.ImageInputs, v)
		out.Workflow[out.Profile.ImageBindings[i].Node].Inputs["image"] = "paneltree/" + v.SHA256 + ".png"
	}
	return nil
}

func (c *ComfyUI) uploadImage(ctx context.Context, input render.ImageInput) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, e := w.CreateFormFile("image", input.SHA256+".png")
	if e != nil {
		return e
	}
	if _, e = part.Write(input.PNG); e != nil {
		return e
	}
	for _, field := range [][2]string{{"type", "input"}, {"subfolder", "paneltree"}, {"overwrite", "true"}} {
		if e = w.WriteField(field[0], field[1]); e != nil {
			return e
		}
	}
	if e = w.Close(); e != nil {
		return e
	}
	data, e := c.httpContent(ctx, http.MethodPost, "/upload/image", body.Bytes(), 2<<20, w.FormDataContentType())
	if e != nil {
		return fmt.Errorf("reference upload: %w", e)
	}
	var result struct {
		Name      string `json:"name"`
		Subfolder string `json:"subfolder"`
		Type      string `json:"type"`
	}
	if e = json.Unmarshal(data, &result); e != nil {
		return e
	}
	if result.Name != input.SHA256+".png" || result.Subfolder != "paneltree" || result.Type != "input" {
		return fmt.Errorf("backend changed reference upload identity")
	}
	return nil
}
