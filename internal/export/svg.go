package export

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/scene"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func decimal(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
func matrix(m scene.Matrix) string {
	parts := make([]string, 6)
	for i, v := range m {
		parts[i] = decimal(v)
	}
	return "matrix(" + strings.Join(parts, " ") + ")"
}
func relative(parent, child scene.Matrix) scene.Matrix {
	d := parent[0]*parent[3] - parent[1]*parent[2]
	inv := scene.Matrix{parent[3] / d, -parent[1] / d, -parent[2] / d, parent[0] / d, 0, 0}
	inv[4], inv[5] = inv.Point(-parent[4], -parent[5])
	return inv.Multiply(child)
}
func (s *Stage) editableSVG(ctx context.Context, resolved scene.Resolved) error {
	var b strings.Builder
	o := resolved.Output()
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, o.Width, o.Height, o.Width, o.Height)
	if s.Page.Background != "" {
		c, err := adapters.ParseColor(s.Page.Background)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#%02x%02x%02x" fill-opacity="%s"/>`, c.R, c.G, c.B, decimal(float64(c.A)/255))
	}
	fmt.Fprintf(&b, `<g transform="%s">`, matrix(o.World))
	fonts := map[string]bool{}
	fontBytes := 0
	var walk func(scene.ResolvedNode, scene.Matrix, string, string) error
	walk = func(n scene.ResolvedNode, parent scene.Matrix, panel, path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if b.Len() > 64<<20 {
			return fmt.Errorf("editable SVG exceeds 64 MiB")
		}
		if n.Kind == "panel" {
			panel = n.ID
		}
		key := path
		if n.Kind == "panel" {
			key = string(s.Page.ID) + "/panel/" + n.ID
		}
		if n.Kind == "layer" {
			key = string(s.Page.ID) + "/panel/" + panel + "/layer/" + n.ID
		}
		id := "pt-" + hex.EncodeToString([]byte(key))
		fmt.Fprintf(&b, `<g id="%s" data-node="%s" data-kind="%s" data-id="%s" transform="%s" opacity="%s">`, id, escape(key), escape(n.Kind), escape(n.ID), matrix(relative(parent, n.World)), decimal(n.Opacity))
		w, h := decimal(n.Bounds.Width), decimal(n.Bounds.Height)
		clip := n.Kind == "panel" || n.Source != nil
		if clip {
			fmt.Fprintf(&b, `<defs><clipPath id="%s-clip"><rect width="%s" height="%s"/></clipPath></defs><g clip-path="url(#%s-clip)">`, id, w, h, id)
		}
		if n.Mask != "" {
			im, err := (adapters.PNG{}).Raster(ctx, model.Source{Kind: "image", Path: n.Mask}, s.Dir)
			if err != nil {
				return err
			}
			white := image.NewNRGBA(im.Bounds())
			for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
				if err := ctx.Err(); err != nil {
					return err
				}
				for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
					_, _, _, a := im.At(x, y).RGBA()
					white.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(a >> 8)})
				}
			}
			var encoded bytes.Buffer
			if err = png.Encode(&encoded, white); err != nil {
				return err
			}
			mask, err := s.store("derived-alpha-mask", n.Mask, "assets/", ".png", encoded.Bytes())
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, `<defs><mask id="%s-mask" maskUnits="userSpaceOnUse" x="0" y="0" width="%s" height="%s"><image href="%s" width="%s" height="%s" preserveAspectRatio="none"/></mask></defs><g mask="url(#%s-mask)">`, id, w, h, escape(mask), w, h, id)
		}
		if n.Source != nil {
			src := n.Source
			if src.Kind == "text" {
				plan, err := adapters.PlanText(ctx, *src, s.Dir, n.Bounds.Width, n.Bounds.Height)
				if err != nil {
					return err
				}
				data, err := adapters.ReadAsset(ctx, s.Dir, src.Font, 8<<20)
				if err != nil {
					return err
				}
				family := "font-" + digest(data)
				if !fonts[family] {
					fontBytes += len(data)
					if fontBytes > 16<<20 {
						return fmt.Errorf("embedded fonts exceed 16 MiB")
					}
					fmt.Fprintf(&b, `<defs><style>@font-face{font-family:%s;src:url(data:font/ttf;base64,%s)} </style></defs>`, family, base64.StdEncoding.EncodeToString(data))
					fonts[family] = true
				}
				c, err := adapters.ParseColor(src.Color)
				if err != nil {
					return err
				}
				for _, line := range plan.Lines {
					xs := []string{}
					for _, x := range line.X {
						xs = append(xs, decimal(x))
					}
					fmt.Fprintf(&b, `<text xml:space="preserve" x="%s" y="%s" font-family="%s" font-size="%s" font-kerning="none" font-variant-ligatures="none" fill="#%02x%02x%02x" fill-opacity="%s">%s</text>`, strings.Join(xs, " "), decimal(line.Baseline), family, decimal(src.FontSize), c.R, c.G, c.B, decimal(float64(c.A)/255), escape(line.Text))
				}
			} else {
				fit := "meet"
				if n.Fit == "cover" {
					fit = "slice"
				}
				fmt.Fprintf(&b, `<image href="%s" width="%s" height="%s" preserveAspectRatio="xMidYMid %s"/>`, escape(src.Path), w, h, fit)
			}
		}
		for i, c := range n.Children {
			if err := walk(c, n.World, panel, key+"/"+c.Kind+"-"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		if n.Mask != "" {
			b.WriteString(`</g>`)
		}
		if clip {
			b.WriteString(`</g>`)
		}
		b.WriteString(`</g>`)
		return nil
	}
	if err := walk(resolved.Tree(), scene.Identity(), "", string(s.Page.ID)); err != nil {
		return err
	}
	b.WriteString(`</g></svg>`)
	return os.WriteFile(filepath.Join(s.Dir, "page.svg"), []byte(b.String()), 0644)
}
