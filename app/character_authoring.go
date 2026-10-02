package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/characters"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"os"
	"path/filepath"
)

// CreateCharacter adds an immutable authored package. Applying it remains explicit.
func (s *Service) CreateCharacter(ctx context.Context, project string, pkg model.CharacterPackage) (string, error) {
	if !referenceID.MatchString(pkg.ID) || !referenceID.MatchString(pkg.Version) {
		return "", fmt.Errorf("valid character ID and version required")
	}
	rel := "characters/authored/" + pkg.ID + "/" + pkg.Version + ".json"
	e := s.openWorkspace(ctx, project, func(w *workspace.Session) error {
		path, e := workspace.SafePath(w.Root, rel)
		if e != nil {
			return e
		}
		data, e := json.Marshal(pkg)
		if e != nil {
			return e
		}
		if len(data) > 1<<20 {
			return fmt.Errorf("character package exceeds limit")
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(data)
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		if e == nil {
			_, e = characters.Resolve(ctx, w.Root, &model.CharacterUse{Package: rel})
		}
		if e != nil {
			_ = os.Remove(path)
		}
		return e
	})
	return rel, e
}
