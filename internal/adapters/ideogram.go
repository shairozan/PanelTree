package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	assets "github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/render"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const IdeogramVersion = "ideogram/v1"

type Ideogram struct {
	APIKey       string
	Client       *http.Client
	BaseURL      string
	PollInterval time.Duration
	Timeout      time.Duration
	Slots        chan struct{}
}

func ValidateIdeogramProfile(p render.GenerationProfile) error {
	if p.Renderer != "ideogram" || p.Model != "ideogram-3" || (p.Operation != "generate" && p.Operation != "character") {
		return fmt.Errorf("unsupported renderer/model/operation")
	}
	switch p.Speed {
	case "turbo", "default", "quality":
	default:
		return fmt.Errorf("invalid rendering-speed")
	}
	switch p.MagicPrompt {
	case "off", "on", "auto":
	default:
		return fmt.Errorf("invalid magic-prompt")
	}
	switch p.StyleType {
	case "auto", "realistic", "fiction":
	default:
		return fmt.Errorf("invalid style-type")
	}
	return nil
}

type IdeogramRecipe struct {
	Adapter     string                   `json:"adapter"`
	ProfileName string                   `json:"profile_name"`
	Profile     render.GenerationProfile `json:"profile"`
	Generation  render.Generation        `json:"generation"`
	Images      []render.ImageInput      `json:"images,omitempty"`
	AspectRatio string                   `json:"aspect_ratio"`
}
type IdeogramResult struct {
	Seed            uint64 `json:"seed"`
	Prompt          string `json:"prompt"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	UsageCostMicros *int64 `json:"usage_cost_usd_micros,omitempty"`
}
type IdeogramState struct {
	Result       *IdeogramResult `json:"result,omitempty"`
	Started      bool            `json:"started"`
	GenerationID string          `json:"generation_id,omitempty"`
}

func FreezeIdeogram(name string, p render.GenerationProfile, r render.Request) (IdeogramRecipe, error) {
	out := IdeogramRecipe{}
	if err := ValidateIdeogramProfile(p); err != nil {
		return out, err
	}
	if r.Generation == nil {
		return out, fmt.Errorf("generation requires a prompt")
	}
	g := *r.Generation
	if r.Character != nil {
		g.Prompt += characterPrompt(r.Character)
	}
	if strings.TrimSpace(g.Prompt) == "" || utf8.RuneCountInString(g.Prompt) > 10000 || utf8.RuneCountInString(g.NegativePrompt) > 10000 || g.Seed > 2147483647 {
		return out, fmt.Errorf("invalid Ideogram prompt or seed (0..2147483647)")
	}
	if g.Output != "" && g.Output != "rgb" {
		return out, fmt.Errorf("ideogram profile supports RGB only")
	}
	g.Output = "rgb"
	w, h := r.Scene.PixelSize.Width, r.Scene.PixelSize.Height
	if math.IsNaN(w) || math.IsNaN(h) || w < 1 || h < 1 || w > 8192 || h > 8192 || w*h > 4<<20 {
		return out, fmt.Errorf("invalid generation dimensions")
	}
	ratios := []string{"1x1", "1x2", "2x1", "2x3", "3x2", "3x4", "4x3", "4x5", "5x4", "9x16", "16x9", "1x3", "3x1", "10x16", "16x10"}
	best, distance := "", math.Inf(1)
	for _, ratio := range ratios {
		var a, b float64
		_, _ = fmt.Sscanf(ratio, "%fx%f", &a, &b)
		d := math.Abs(math.Log(w / h / (a / b)))
		if d < distance {
			best, distance = ratio, d
		}
	}
	out = IdeogramRecipe{Adapter: IdeogramVersion, ProfileName: name, Profile: p, Generation: g, AspectRatio: best}
	characters, styles, total := 0, 0, 0
	for _, im := range r.ImageInputs {
		switch im.Role {
		case "character":
			characters++
		case "style":
			styles++
		default:
			return out, fmt.Errorf("ideogram requires explicit character or style image roles")
		}
		total += len(im.PNG)
		if len(im.PNG) > 25<<20 || total > 32<<20 || im.SHA256 != assets.Digest(im.PNG) {
			return out, fmt.Errorf("invalid reference hash or size")
		}
		cfg, e := png.DecodeConfig(bytes.NewReader(im.PNG))
		if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
			return out, fmt.Errorf("reference must be a PNG within four megapixels")
		}
		if _, e = png.Decode(bytes.NewReader(im.PNG)); e != nil {
			return out, fmt.Errorf("invalid reference PNG")
		}
		im.PNG = bytes.Clone(im.PNG)
		out.Images = append(out.Images, im)
	}
	if styles > 10 || characters > 1 || (p.Operation == "character" && characters != 1) || (p.Operation == "generate" && characters != 0) {
		return out, fmt.Errorf("profile requires %s inputs: character operation needs exactly one character; generate accepts style only", p.Operation)
	}
	return out, nil
}

type ideogramResponse struct {
	UsageCostMicros *int64 `json:"usage_cost_usd_micros,omitempty"`
	GenerationID    string `json:"generation_id"`
	Status          string `json:"status"`
	Data            []struct {
		URL        string `json:"url"`
		Safe       bool   `json:"is_image_safe"`
		Seed       uint64 `json:"seed"`
		Prompt     string `json:"prompt"`
		Resolution string `json:"resolution"`
	} `json:"data"`
}

var remoteID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,256}$`)

func (c *Ideogram) endpoint(r IdeogramRecipe) string {
	base := c.BaseURL
	if base == "" {
		base = "https://api.ideogram.ai"
	}
	path := "ideogram-3"
	if r.Profile.Operation == "character" {
		path += "-character"
	}
	return base + "/v2/image/generate/" + path
}
func (c *Ideogram) client() *http.Client {
	client := http.Client{Timeout: 30 * time.Second}
	if c.Client != nil {
		client = *c.Client
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}
func (c *Ideogram) request(ctx context.Context, method, address, contentType string, body io.Reader, auth bool) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, address, body)
	if e != nil {
		return nil, fmt.Errorf("invalid Ideogram request")
	}
	if auth {
		req.Header.Set("Api-Key", c.APIKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, e := c.client().Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("ideogram transport failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ideogram HTTP %d", response.StatusCode)
	}
	limit := int64(1 << 20)
	if !auth {
		limit = 32 << 20
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil {
		return nil, fmt.Errorf("ideogram response read failed")
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("ideogram response too large")
	}
	return data, nil
}
func ideogramBody(r IdeogramRecipe) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{"prompt": r.Generation.Prompt, "negative_prompt": r.Generation.NegativePrompt, "seed": strconv.FormatUint(r.Generation.Seed, 10), "aspect_ratio": r.AspectRatio, "rendering_speed": r.Profile.Speed, "magic_prompt": r.Profile.MagicPrompt, "style_type": r.Profile.StyleType, "num_images": "1", "async": "true"}
	for k, v := range fields {
		if e := w.WriteField(k, v); e != nil {
			return nil, "", e
		}
	}
	for _, im := range r.Images {
		field := "style_reference_images"
		if im.Role == "character" {
			field = "character_reference_images"
		}
		part, e := w.CreateFormFile(field, im.SHA256+".png")
		if e != nil {
			return nil, "", e
		}
		if _, e = part.Write(im.PNG); e != nil {
			return nil, "", e
		}
	}
	if e := w.Close(); e != nil {
		return nil, "", e
	}
	return &buf, w.FormDataContentType(), nil
}
func (c *Ideogram) Run(ctx context.Context, r IdeogramRecipe, state IdeogramState, save func(IdeogramState) error) ([]byte, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ideogram API key is not configured")
	}
	if r.Adapter != IdeogramVersion {
		return nil, fmt.Errorf("unsupported Ideogram recipe")
	}
	if e := ValidateIdeogramProfile(r.Profile); e != nil {
		return nil, e
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if c.Slots != nil {
		select {
		case c.Slots <- struct{}{}:
			defer func() { <-c.Slots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if state.GenerationID == "" {
		if state.Started {
			return nil, fmt.Errorf("submission outcome unknown; refusing to repeat a possibly billed request")
		}
		body, ct, e := ideogramBody(r)
		if e != nil {
			return nil, e
		}
		state.Started = true
		if e = save(state); e != nil {
			return nil, e
		}
		data, e := c.request(ctx, http.MethodPost, c.endpoint(r), ct, body, true)
		if e != nil {
			return nil, e
		}
		var ack ideogramResponse
		if e = json.Unmarshal(data, &ack); e != nil || !remoteID.MatchString(ack.GenerationID) {
			return nil, fmt.Errorf("invalid submission acknowledgement; outcome unknown")
		}
		state.GenerationID = ack.GenerationID
		if e = save(state); e != nil {
			return nil, e
		}
	}
	if !remoteID.MatchString(state.GenerationID) {
		return nil, fmt.Errorf("invalid remote generation ID")
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.ideogram.ai"
	}
	interval := c.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	for {
		data, e := c.request(ctx, http.MethodGet, base+"/v2/generations/"+state.GenerationID, "", nil, true)
		if e != nil {
			return nil, e
		}
		var response ideogramResponse
		if e = json.Unmarshal(data, &response); e != nil {
			return nil, fmt.Errorf("invalid polling response")
		}
		if response.GenerationID != state.GenerationID {
			return nil, fmt.Errorf("remote generation identity mismatch")
		}
		switch response.Status {
		case "completed":
			if len(response.Data) != 1 || !response.Data[0].Safe {
				return nil, fmt.Errorf("ideogram returned no single safe image")
			}
			u, e := url.Parse(response.Data[0].URL)
			if e != nil || u.User != nil {
				return nil, fmt.Errorf("invalid image URL")
			}
			// Credentials never leave the API endpoint. Restrict downloads to provider storage.
			allowed := u.Scheme == "https" && (u.Hostname() == "ideogram.ai" || strings.HasSuffix(u.Hostname(), ".ideogram.ai") || u.Hostname() == "storage.googleapis.com") && u.Port() == ""
			if c.BaseURL != "" {
				testURL, _ := url.Parse(c.BaseURL)
				allowed = allowed || (u.Scheme == testURL.Scheme && u.Host == testURL.Host)
			}
			if !allowed {
				return nil, fmt.Errorf("untrusted Ideogram image host")
			}
			imageData, e := c.request(ctx, http.MethodGet, u.String(), "", nil, false)
			if e != nil {
				return nil, e
			}
			cfg, _, e := image.DecodeConfig(bytes.NewReader(imageData))
			if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
				return nil, fmt.Errorf("invalid generated image dimensions")
			}
			im, _, e := image.Decode(bytes.NewReader(imageData))
			if e != nil {
				return nil, fmt.Errorf("invalid generated image")
			}
			var out bytes.Buffer
			if e = png.Encode(&out, im); e != nil {
				return nil, e
			}
			state.Result = &IdeogramResult{Seed: response.Data[0].Seed, Prompt: response.Data[0].Prompt, Width: cfg.Width, Height: cfg.Height, UsageCostMicros: response.UsageCostMicros}
			if e = save(state); e != nil {
				return nil, e
			}
			return out.Bytes(), nil
		case "failed":
			return nil, fmt.Errorf("ideogram generation failed")
		case "pending":
		default:
			return nil, fmt.Errorf("unsupported Ideogram generation status")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Ideogram) Estimate(ctx context.Context, r IdeogramRecipe) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("ideogram API key is not configured")
	}
	if r.Adapter != IdeogramVersion {
		return nil, fmt.Errorf("unsupported Ideogram recipe")
	}
	if e := ValidateIdeogramProfile(r.Profile); e != nil {
		return nil, e
	}
	body, ct, e := ideogramBody(r)
	if e != nil {
		return nil, e
	}
	data, e := c.request(ctx, http.MethodPost, c.endpoint(r)+"?dry_run=true", ct, body, true)
	if e != nil {
		return nil, e
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid Ideogram price quote")
	}
	return json.RawMessage(data), nil
}
