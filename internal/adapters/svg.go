package adapters

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strconv"
	"strings"
)

type shape struct {
	kind       string
	x, y, w, h float64
	points     []float64
	fill       color.NRGBA
}
type svgDoc struct {
	width, height float64
	shapes        []shape
}

func parseSVG(data []byte) (svgDoc, error) {
	var out svgDoc
	dec := xml.NewDecoder(bytes.NewReader(data))
	stack := []string{}
	fills := []color.NRGBA{}
	count := 0
	rootSeen := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		switch t := tok.(type) {
		case xml.Directive:
			return out, fmt.Errorf("SVG directives unsupported")
		case xml.ProcInst:
			if t.Target != "xml" {
				return out, fmt.Errorf("SVG processing instructions unsupported")
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return out, fmt.Errorf("SVG text unsupported; use lettering source")
			}
		case xml.StartElement:
			count++
			if count > 256 || len(stack) > 32 {
				return out, fmt.Errorf("SVG exceeds 256 elements or depth 32")
			}
			if t.Name.Space != "http://www.w3.org/2000/svg" {
				return out, fmt.Errorf("SVG requires xmlns=\"http://www.w3.org/2000/svg\"")
			}
			kind := t.Name.Local
			allowed := "id fill"
			switch kind {
			case "svg":
				if rootSeen || len(stack) != 0 {
					return out, fmt.Errorf("nested/multiple SVG roots unsupported")
				}
				rootSeen = true
				allowed = "xmlns width height viewBox"
			case "g":
			case "rect":
				allowed += " x y width height opacity"
			case "circle":
				allowed += " cx cy r opacity"
			case "ellipse":
				allowed += " cx cy rx ry opacity"
			case "polygon":
				allowed += " points opacity"
			default:
				return out, fmt.Errorf("unsupported SVG element %s", kind)
			}
			if len(stack) == 0 && kind != "svg" {
				return out, fmt.Errorf("expected SVG root")
			}
			if len(stack) > 0 && stack[len(stack)-1] != "svg" && stack[len(stack)-1] != "g" {
				return out, fmt.Errorf("shape cannot have children")
			}
			attrs := map[string]string{}
			for _, a := range t.Attr {
				if a.Name.Space != "" || !strings.Contains(" "+allowed+" ", " "+a.Name.Local+" ") {
					return out, fmt.Errorf("unsupported SVG %s attribute %s", kind, a.Name.Local)
				}
				if _, ok := attrs[a.Name.Local]; ok {
					return out, fmt.Errorf("duplicate SVG attribute")
				}
				attrs[a.Name.Local] = a.Value
			}
			f := color.NRGBA{A: 255}
			if len(fills) > 0 {
				f = fills[len(fills)-1]
			}
			if v, ok := attrs["fill"]; ok {
				f, err = ParseColor(v)
				if err != nil {
					return out, err
				}
			}
			if kind == "svg" {
				if v, ok := attrs["xmlns"]; ok && v != "http://www.w3.org/2000/svg" {
					return out, fmt.Errorf("invalid SVG namespace")
				}
				out.width, err = scalar(attrs["width"])
				if err != nil {
					return out, err
				}
				out.height, err = scalar(attrs["height"])
				if err != nil {
					return out, err
				}
				if out.width <= 0 || out.height <= 0 || out.width > 8192 || out.height > 8192 {
					return out, fmt.Errorf("invalid SVG dimensions")
				}
				if v, ok := attrs["viewBox"]; ok {
					p, e := numbers(v)
					if e != nil || len(p) != 4 || p[0] != 0 || p[1] != 0 || p[2] != out.width || p[3] != out.height {
						return out, fmt.Errorf("viewBox must be 0 0 width height")
					}
				}
			} else if kind != "g" {
				s := shape{kind: kind, fill: f}
				num := func(key string) float64 {
					v, ok := attrs[key]
					if !ok {
						return 0
					}
					n, e := scalar(v)
					if e != nil {
						err = e
					}
					return n
				}
				switch kind {
				case "rect":
					s.x = num("x")
					s.y = num("y")
					s.w = num("width")
					s.h = num("height")
				case "circle", "ellipse":
					s.x = num("cx")
					s.y = num("cy")
					if kind == "circle" {
						s.w = num("r")
						s.h = s.w
					} else {
						s.w = num("rx")
						s.h = num("ry")
					}
				case "polygon":
					s.points, err = numbers(attrs["points"])
					if len(s.points) < 6 || len(s.points) > 512 || len(s.points)%2 != 0 {
						return out, fmt.Errorf("polygon needs 3..256 point pairs")
					}
				}
				if err != nil {
					return out, err
				}
				if kind != "polygon" && (s.w <= 0 || s.h <= 0) {
					return out, fmt.Errorf("shape dimensions must be positive")
				}
				if _, ok := attrs["opacity"]; ok {
					o := num("opacity")
					if err != nil || o < 0 || o > 1 {
						return out, fmt.Errorf("invalid opacity")
					}
					s.fill.A = uint8(math.Round(float64(s.fill.A) * o))
				}
				out.shapes = append(out.shapes, s)
			}
			stack = append(stack, kind)
			fills = append(fills, f)
		case xml.EndElement:
			if len(stack) == 0 {
				return out, fmt.Errorf("unbalanced SVG")
			}
			stack = stack[:len(stack)-1]
			fills = fills[:len(fills)-1]
		}
	}
	if !rootSeen || len(stack) != 0 {
		return out, fmt.Errorf("incomplete SVG")
	}
	return out, nil
}
func scalar(s string) (float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
		return 0, fmt.Errorf("invalid SVG number %q", s)
	}
	return v, nil
}
func numbers(s string) ([]float64, error) {
	parts := strings.Fields(strings.ReplaceAll(s, ",", " "))
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, e := scalar(p)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func rasterSVG(ctx context.Context, r render.Request) (image.Image, error) {
	b, err := ReadAsset(ctx, r.BaseDir, r.Source.Path, 1<<20)
	if err != nil {
		return nil, err
	}
	doc, err := parseSVG(b)
	if err != nil {
		return nil, fmt.Errorf("SVG %s: %w", r.Source.Path, err)
	}
	scale := 1.0
	if r.Scene.PixelSize.Width > 0 && r.Scene.PixelSize.Height > 0 {
		scale = math.Min(r.Scene.PixelSize.Width/doc.width, r.Scene.PixelSize.Height/doc.height)
		if r.Fit == "cover" {
			scale = math.Max(r.Scene.PixelSize.Width/doc.width, r.Scene.PixelSize.Height/doc.height)
		}
	}
	w, h, err := rasterSize(doc.width*scale, doc.height*scale)
	if err != nil {
		return nil, err
	}
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, s := range doc.shapes {
		for y := 0; y < h; y++ {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			for x := 0; x < w; x++ {
				if s.contains((float64(x)+.5)*doc.width/float64(w), (float64(y)+.5)*doc.height/float64(h)) {
					draw.Draw(im, image.Rect(x, y, x+1, y+1), image.NewUniform(s.fill), image.Point{}, draw.Over)
				}
			}
		}
	}
	return im, nil
}
func (s shape) contains(x, y float64) bool {
	switch s.kind {
	case "rect":
		return x >= s.x && y >= s.y && x < s.x+s.w && y < s.y+s.h
	case "circle", "ellipse":
		dx, dy := (x-s.x)/s.w, (y-s.y)/s.h
		return dx*dx+dy*dy <= 1
	case "polygon":
		winding := 0
		j := len(s.points) - 2
		for i := 0; i < len(s.points); i += 2 {
			xi, yi, xj, yj := s.points[i], s.points[i+1], s.points[j], s.points[j+1]
			cross := (xj-xi)*(y-yi) - (x-xi)*(yj-yi)
			if yi <= y && yj > y && cross > 0 {
				winding++
			}
			if yi > y && yj <= y && cross < 0 {
				winding--
			}
			j = i
		}
		return winding != 0
	}
	return false
}
