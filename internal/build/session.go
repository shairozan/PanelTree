// Package build plans content-addressed production, composition and export nodes.
package build

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"runtime"

	"github.com/shairozan/PanelTree/internal/cache"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"github.com/shairozan/PanelTree/scene"
)

type Event struct {
	Kind, ID, Recipe, ContentHash, Decision, Reason string
	Inputs                                          []string
}
type Report struct {
	LeafRenders, Recompositions, Encodes, CacheHits int
	Events                                          []Event
}
type Session struct {
	Store      *cache.Store
	Rasterizer render.CacheRasterizer
	Base       string
	Output     scene.Output
	Report     Report
	nodes      map[string]int
	assets     map[string]int
	root       string
}

func New(ctx context.Context, store *cache.Store, raster render.CacheRasterizer, base string, resolved scene.Resolved) (*Session, error) {
	s := &Session{Store: store, Rasterizer: raster, Base: base, Output: resolved.Output(), nodes: map[string]int{}, assets: map[string]int{}}
	var err error
	s.root, err = s.plan(ctx, resolved.Tree())
	return s, err
}
func request(n scene.ResolvedNode, o scene.Output, base string) render.Request {
	world := o.World.Multiply(n.World)
	return render.Request{Source: *n.Source, Fit: n.Fit, BaseDir: base, Scene: scene.Context{Bounds: n.Bounds, PixelSize: model.Canvas{Width: n.Bounds.Width * math.Hypot(world[0], world[1]), Height: n.Bounds.Height * math.Hypot(world[2], world[3])}}}
}
func (s *Session) asset(ctx context.Context, r render.Request) (string, error) {
	recipe, err := s.Rasterizer.CacheRecipe(ctx, r)
	if err != nil {
		return "", err
	}
	key, err := cache.Key(struct {
		Type   string
		Recipe any
	}{"asset/v2", recipe})
	if err != nil {
		return "", err
	}
	if _, ok := s.assets[key]; !ok {
		s.assets[key] = len(s.Report.Events)
		s.Report.Events = append(s.Report.Events, Event{Kind: "asset", ID: r.Source.Kind, Recipe: key, Decision: "skipped", Reason: "ancestor reused"})
	}
	return key, nil
}
func (s *Session) plan(ctx context.Context, n scene.ResolvedNode) (string, error) {
	identity, err := cache.Key(n)
	if err != nil {
		return "", err
	}
	clean := n
	clean.Children = nil
	clean.Source = nil
	clean.Mask = ""
	var inputs []string
	if n.Source != nil {
		key, e := s.asset(ctx, request(n, s.Output, s.Base))
		if e != nil {
			return "", e
		}
		inputs = append(inputs, key)
	}
	if n.Mask != "" {
		key, e := s.asset(ctx, render.Request{Source: model.Source{Kind: "image", Path: n.Mask}, BaseDir: s.Base})
		if e != nil {
			return "", e
		}
		inputs = append(inputs, key)
	}
	for _, child := range n.Children {
		key, e := s.plan(ctx, child)
		if e != nil {
			return "", e
		}
		inputs = append(inputs, key)
	}
	key, err := cache.Key(struct {
		Version string
		Node    scene.ResolvedNode
		Output  scene.Output
		Inputs  []string
	}{"compose/v2/" + runtime.Version(), clean, s.Output, inputs})
	if err != nil {
		return "", err
	}
	s.nodes[identity] = len(s.Report.Events)
	s.Report.Events = append(s.Report.Events, Event{Kind: n.Kind, ID: n.ID, Recipe: key, Inputs: inputs, Decision: "skipped", Reason: "ancestor reused"})
	return key, nil
}
func (s *Session) load(ctx context.Context, index int, w, h int) (image.Image, error) {
	event := &s.Report.Events[index]
	entry, err := s.Store.Get(ctx, event.Recipe)
	if err != nil {
		return nil, err
	}
	event.Reason = entry.Reason
	event.Decision = "miss"
	if !entry.Hit {
		return nil, nil
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(entry.Data))
	raw := len(entry.Data) >= 12 && string(entry.Data[:4]) == "RGBA"
	if raw {
		cfg.Width = int(binary.BigEndian.Uint32(entry.Data[4:8]))
		cfg.Height = int(binary.BigEndian.Uint32(entry.Data[8:12]))
		err = nil
	}
	valid := err == nil && cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= 8192 && cfg.Height <= 8192 && int64(cfg.Width)*int64(cfg.Height) <= 16<<20
	if w > 0 {
		valid = valid && cfg.Width == w && cfg.Height == h
	} else {
		valid = valid && int64(cfg.Width)*int64(cfg.Height) <= 4<<20
	}
	var im image.Image
	if valid {
		if raw {
			valid = int64(len(entry.Data)-12) == int64(cfg.Width)*int64(cfg.Height)*4
			if valid {
				im = &image.RGBA{Pix: entry.Data[12:], Stride: cfg.Width * 4, Rect: image.Rect(0, 0, cfg.Width, cfg.Height)}
			}
		} else {
			im, err = png.Decode(bytes.NewReader(entry.Data))
			valid = err == nil
		}
	}
	if !valid {
		if err = s.Store.Invalidate(event.Recipe); err != nil {
			return nil, err
		}
		event.Reason = "invalid-raster"
		return nil, nil
	}
	event.Decision = "hit"
	event.ContentHash = entry.ContentHash
	s.Report.CacheHits++
	return im, nil
}
func (s *Session) save(ctx context.Context, index int, im image.Image) error {
	var buf bytes.Buffer
	if rgba, ok := im.(*image.RGBA); ok && s.Report.Events[index].Kind != "export" {
		// PNG unpremultiplies and quantizes semi-transparent pixels. Preserve
		// composition intermediates byte-for-byte in a bounded RGBA envelope.
		header := make([]byte, 12)
		copy(header, "RGBA")
		binary.BigEndian.PutUint32(header[4:8], uint32(rgba.Rect.Dx()))
		binary.BigEndian.PutUint32(header[8:12], uint32(rgba.Rect.Dy()))
		buf.Write(header)
		for y := rgba.Rect.Min.Y; y < rgba.Rect.Max.Y; y++ {
			i := rgba.PixOffset(rgba.Rect.Min.X, y)
			buf.Write(rgba.Pix[i : i+rgba.Rect.Dx()*4])
		}
	} else {
		if err := png.Encode(&buf, im); err != nil {
			return err
		}
	}
	hash, err := s.Store.Put(ctx, s.Report.Events[index].Recipe, buf.Bytes())
	if err != nil {
		return err
	}
	event := &s.Report.Events[index]
	event.Decision = "built"
	event.ContentHash = hash
	return nil
}
func (s *Session) Load(ctx context.Context, n scene.ResolvedNode) (*image.RGBA, error) {
	key, err := cache.Key(n)
	if err != nil {
		return nil, err
	}
	index, ok := s.nodes[key]
	if !ok {
		return nil, fmt.Errorf("unplanned node")
	}
	im, err := s.load(ctx, index, s.Output.Width, s.Output.Height)
	if err != nil || im == nil {
		return nil, err
	}
	if rgba, ok := im.(*image.RGBA); ok {
		return rgba, nil
	}
	rgba := image.NewRGBA(im.Bounds())
	draw.Draw(rgba, rgba.Bounds(), im, image.Point{}, draw.Src)
	return rgba, nil
}
func (s *Session) Save(ctx context.Context, n scene.ResolvedNode, im *image.RGBA) error {
	key, err := cache.Key(n)
	if err != nil {
		return err
	}
	index, ok := s.nodes[key]
	if !ok {
		return fmt.Errorf("unplanned node")
	}
	if err = s.save(ctx, index, im); err != nil {
		return err
	}
	s.Report.Recompositions++
	return nil
}
func (s *Session) Raster(ctx context.Context, source model.Source, base string) (image.Image, error) {
	return s.RasterScene(ctx, render.Request{Source: source, BaseDir: base})
}
func (s *Session) RasterScene(ctx context.Context, r render.Request) (image.Image, error) {
	key, err := s.asset(ctx, r)
	if err != nil {
		return nil, err
	}
	index := s.assets[key]
	im, err := s.load(ctx, index, 0, 0)
	if err != nil || im != nil {
		return im, err
	}
	im, err = s.Rasterizer.RasterScene(ctx, r)
	if err != nil {
		return nil, err
	}
	if im == nil || im.Bounds().Empty() {
		return nil, fmt.Errorf("empty raster")
	}
	if err = s.save(ctx, index, im); err != nil {
		return nil, err
	}
	s.Report.LeafRenders++
	return im, nil
}

// Export has its own recipe: background affects the final page, not transparent
// child compositions. The encoded bytes are reused; publishing is always fresh.
func (s *Session) Export(ctx context.Context, im image.Image, background string) ([]byte, string, error) {
	key, err := cache.Key(struct{ Version, Input, Background string }{"png/v1/" + runtime.Version(), s.root, background})
	if err != nil {
		return nil, "", err
	}
	index := len(s.Report.Events)
	s.Report.Events = append(s.Report.Events, Event{Kind: "export", Recipe: key, Inputs: []string{s.root}})
	hit, err := s.load(ctx, index, s.Output.Width, s.Output.Height)
	if err != nil {
		return nil, "", err
	}
	if hit == nil {
		if err = s.save(ctx, index, im); err != nil {
			return nil, "", err
		}
		s.Report.Encodes++
	}
	entry, err := s.Store.Get(ctx, key)
	if err != nil {
		return nil, "", err
	}
	if !entry.Hit {
		return nil, "", fmt.Errorf("export disappeared during build")
	}
	return entry.Data, key, nil
}
