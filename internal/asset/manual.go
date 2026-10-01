package asset

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/internal/workspace"
	"os"
	"path/filepath"
)

// ManualPath resolves a portable snapshot when present, otherwise preserves
// the live-file semantics of existing filesystem overrides.
func ManualPath(root, original string) (string, error) {
	refs, e := ManualReferences(root)
	if e != nil {
		return "", e
	}
	if hash := refs[original]; hash != "" {
		return Resolve(root, hash)
	}
	if filepath.IsAbs(original) {
		return original, nil
	}
	return workspace.SafePath(root, original)
}
func ManualReferences(root string) (map[string]string, error) {
	path, e := workspace.SafePath(root, ".paneltree/manual-assets.json")
	if e != nil {
		return nil, e
	}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return map[string]string{}, nil
	}
	if e != nil {
		return nil, e
	}
	var out map[string]string
	e = json.Unmarshal(b, &out)
	if out == nil {
		out = map[string]string{}
	}
	return out, e
}
