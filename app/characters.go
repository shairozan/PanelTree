package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/characters"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
)

func currentCharacter(ctx context.Context, w *workspace.Session, target LayerTarget) (*model.ResolvedCharacter, error) {
	n, ok := indexLayers(w.Snapshot)[targetKey(target)]
	if !ok || n.layer.Source == nil {
		return nil, fmt.Errorf("character target no longer exists")
	}
	return characters.Resolve(ctx, w.Root, n.layer.Source.Character)
}

// Dependency status is observational: it never clears pins, mutates a job, or
// changes authored YAML. Frozen provenance remains visible even on read errors.
func (s *Service) characterStatuses(ctx context.Context, project string, list []Job) error {
	needed := false
	for _, j := range list {
		if j.GenerationProvenance != nil && j.GenerationProvenance.Character != nil {
			needed = true
		}
	}
	if !needed {
		return nil
	}
	return workspace.Open(ctx, project, func(w *workspace.Session) error {
		store, e := jobStore(w)
		if e != nil {
			return e
		}
		for i := range list {
			j := &list[i]
			if j.GenerationProvenance == nil || j.GenerationProvenance.Character == nil {
				continue
			}
			_, payload, e := store.Lookup(ctx, j.Key)
			if e != nil {
				return e
			}
			var input assetInput
			if e = json.Unmarshal(payload, &input); e != nil {
				return e
			}
			current, e := currentCharacter(ctx, w, input.Target)
			switch {
			case e != nil:
				j.CharacterStatus = "unavailable"
				j.CharacterDiagnostic = e.Error()
			case hash(current) != hash(j.GenerationProvenance.Character):
				j.CharacterStatus = "changed"
			default:
				j.CharacterStatus = "current"
			}
		}
		return nil
	})
}
