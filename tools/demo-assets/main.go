// Command demo-assets regenerates PanelTree's original CC0 geometric PNG fixtures.
package main

import (
	"golang.org/x/image/font/gofont/goregular"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	dir := "internal/project/template/assets"
	if err := os.WriteFile(filepath.Join(dir, "Go-Regular.ttf"), goregular.TTF, 0644); err != nil {
		panic(err)
	}
	setting := image.NewNRGBA(image.Rect(0, 0, 600, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 600; x++ {
			c := color.NRGBA{R: uint8(18 + y/10), G: uint8(32 + y/8), B: uint8(63 + y/7), A: 255}
			if (x-440)*(x-440)+(y-130)*(y-130) < 65*65 {
				c = color.NRGBA{R: 245, G: 197, B: 109, A: 255}
			}
			if y > 320+(x%173)/3 {
				c = color.NRGBA{R: 38, G: 69, B: 85, A: 255}
			}
			if y > 430+(x%131)/4 {
				c = color.NRGBA{R: 21, G: 42, B: 58, A: 255}
			}
			if y > 545 {
				c = color.NRGBA{R: 13, G: 28, B: 43, A: 255}
			}
			setting.SetNRGBA(x, y, c)
		}
	}
	hero := image.NewNRGBA(image.Rect(0, 0, 240, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 240; x++ {
			c := color.NRGBA{}
			if y > 180 && y < 395 && x > 40+(y-180)/10 && x < 200-(y-180)/10 {
				c = color.NRGBA{R: 231, G: 103, B: 75, A: 255}
			}
			if (x-120)*(x-120)+(y-130)*(y-130) < 52*52 {
				c = color.NRGBA{R: 237, G: 205, B: 159, A: 255}
			}
			if y > 83 && y < 115 && x > 75 && x < 166 {
				c = color.NRGBA{R: 29, G: 32, B: 45, A: 255}
			}
			if y > 385 && y < 467 && ((x > 67 && x < 103) || (x > 137 && x < 173)) {
				c = color.NRGBA{R: 32, G: 47, B: 67, A: 255}
			}
			if y > 133 && y < 139 && ((x > 95 && x < 105) || (x > 135 && x < 145)) {
				c = color.NRGBA{R: 29, G: 32, B: 45, A: 255}
			}
			hero.SetNRGBA(x, y, c)
		}
	}
	mask := image.NewNRGBA(image.Rect(0, 0, 600, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 600; x++ {
			if (x-300)*(x-300)+(y-300)*(y-300) < 290*290 {
				mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	for name, im := range map[string]image.Image{"setting.png": setting, "hero.png": hero, "iris.png": mask} {
		if err := save(filepath.Join(dir, name), im); err != nil {
			panic(err)
		}
	}
}
func save(path string, im image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(f, im); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
