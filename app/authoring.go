package app

import (
	"bytes"
	"context"
	"fmt"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"image"
	"image/png"
	"os"
)

// AuthoringAssets installs immutable defaults without changing any existing source.
// Font licensing travels with the project, including PostgreSQL round trips.
func (s *Service) AuthoringAssets(ctx context.Context, handle string) (map[string]string, error) {
	out := map[string]string{}
	err := s.openWorkspace(ctx, handle, func(w *workspace.Session) error {
		font, license, e := project.DefaultFontFiles()
		if e != nil {
			return e
		}
		rel := ".paneltree/assets/" + asset.Digest(font) + ".ttf"
		var pngData bytes.Buffer
		if e = png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1, 1))); e != nil {
			return e
		}
		id, e := asset.Put(w.Root, pngData.Bytes())
		if e != nil {
			return e
		}
		for name, data := range map[string][]byte{rel: font, rel + ".LICENSE": license} {
			path, e := workspace.SafePath(w.Root, name)
			if e != nil {
				return e
			}
			f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if os.IsExist(e) {
				old, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !bytes.Equal(old, data) {
					return fmt.Errorf("authoring asset conflicts with existing file")
				}
				continue
			}
			if e != nil {
				return e
			}
			_, e = f.Write(data)
			closeErr := f.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
		}
		out["font"] = rel
		out["blank"] = ".paneltree/assets/" + id + ".png"
		return nil
	})
	return out, err
}
