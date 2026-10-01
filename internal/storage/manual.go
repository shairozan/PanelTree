package storage

import (
	"encoding/json"
	"github.com/shairozan/PanelTree/internal/asset"
)

// Keep original editorial state bytes (and lock fingerprints) while freezing
// selected manual artwork independently of its old machine-local filename.
func captureManual(root string, b *bundle) error {
	var states map[string]struct {
		Manual string `json:"manual"`
	}
	if raw := b.Files[".paneltree/state.json"]; len(raw) > 0 {
		if e := json.Unmarshal(raw, &states); e != nil {
			return e
		}
	}
	refs := map[string]string{}
	for _, st := range states {
		if st.Manual == "" {
			continue
		}
		path, e := asset.ManualPath(root, st.Manual)
		if e != nil {
			return e
		}
		data, e := asset.ReadPNG(path)
		if e != nil {
			return e
		}
		hash := asset.Digest(data)
		refs[st.Manual] = hash
		b.Files[".paneltree/assets/"+hash+".png"] = data
	}
	if len(refs) > 0 {
		data, e := json.Marshal(refs)
		if e != nil {
			return e
		}
		b.Files[".paneltree/manual-assets.json"] = data
	}
	return nil
}
