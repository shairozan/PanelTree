package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/asset"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DocumentEdit = workspace.Edit
type LayerTarget struct {
	Page  model.ID `json:"page"`
	Panel model.ID `json:"panel"`
	Layer model.ID `json:"layer"`
}
type Operation struct {
	Candidate string          `json:"candidate,omitempty"`
	Target    LayerTarget     `json:"target"`
	Action    string          `json:"action"`
	Scope     model.LockScope `json:"scope,omitempty"`
	Artifact  string          `json:"artifact,omitempty"`
}
type EditRequest struct {
	ProjectFile      string         `json:"-"`
	ExpectedRevision model.Revision `json:"expected_revision"`
	Edits            []DocumentEdit `json:"edits,omitempty"`
	Operations       []Operation    `json:"operations,omitempty"`
}
type LayerStatus struct {
	State     model.EditorialState `json:"state"`
	Pin       string               `json:"pin,omitempty"`
	Manual    string               `json:"manual,omitempty"`
	Request   string               `json:"request,omitempty"`
	Lock      model.LockScope      `json:"lock,omitempty"`
	Asset     string               `json:"asset,omitempty"`
	Placement string               `json:"placement,omitempty"`
	Stale     bool                 `json:"stale,omitempty"`
}
type EditResult struct {
	Revision model.Revision         `json:"revision"`
	Layers   map[string]LayerStatus `json:"layers"`
}

func (s *Service) Edit(ctx context.Context, r EditRequest) (EditResult, error) {
	var result EditResult
	err := workspace.Open(ctx, r.ProjectFile, func(w *workspace.Session) error {
		if _, e := os.Stat(filepath.Join(w.Root, "project.yaml")); e == nil && w.Entry != filepath.Join(w.Root, "project.yaml") {
			return fmt.Errorf("edit the owning project.yaml to enforce project-wide policy")
		}
		states, e := readStates(w.State)
		if e != nil {
			return e
		}
		w.Validate = func(next *project.Snapshot) ([]byte, error) {
			old := indexLayers(w.Snapshot)
			nodes := indexLayers(next)
			checked := make(map[string]LayerStatus, len(states))
			for key, st := range states {
				checked[key] = st
			}
			if len(r.Edits) == 0 {
				onlyUnlock := true
				for _, op := range r.Operations {
					if op.Action != "unlock" {
						onlyUnlock = false
					}
				}
				if onlyUnlock {
					for _, op := range r.Operations {
						key := targetKey(op.Target)
						st := checked[key]
						st.Lock = ""
						checked[key] = st
					}
				}
			}
			// Unlock is a separate transaction, so ordering operations cannot bypass locks.
			if e := checkLocks(old, nodes, checked, w.Root); e != nil {
				return nil, e
			}
			seen := map[string]bool{}
			for _, op := range r.Operations {
				key := targetKey(op.Target)
				n, ok := nodes[key]
				if !ok {
					return nil, fmt.Errorf("unknown layer %s", key)
				}
				if seen[key] {
					return nil, fmt.Errorf("one operation per layer per changeset")
				}
				seen[key] = true
				if op.Action == "approve" || op.Action == "override" || op.Action == "clear-selection" || op.Action == "select-candidate" {
					for owner, locked := range checked {
						if locked.Lock != model.AssetLock && locked.Lock != model.AllLock {
							continue
						}
						if containsLayer(nodes[owner].layer, n.layer) {
							return nil, fmt.Errorf("asset lock on %s protects selection %s", owner, key)
						}
					}
				}
				st := states[key]
				if st.State == "" {
					st.State = model.Draft
				}
				if st.Lock != "" && op.Action == "lock" {
					return nil, fmt.Errorf("layer %s requires explicit unlock", key)
				}
				switch op.Action {
				case "select-candidate":
					if len(r.Edits) != 0 || len(r.Operations) != 1 {
						return nil, fmt.Errorf("candidate selection requires its own changeset")
					}
					if st.State == model.Approved || st.Manual != "" {
						return nil, fmt.Errorf("clear approved/manual selection explicitly before selecting a candidate")
					}
					pin, e := s.candidate(ctx, w, op)
					if e != nil {
						return nil, e
					}
					st.Pin = pin
					st.Manual = ""
					st.State = model.Draft
					st.Request, e = requestIdentity(n)
					if e != nil {
						return nil, e
					}
				case "lock":
					scope := op.Scope
					if scope == "" {
						scope = model.AllLock
					}
					if scope != model.AllLock && scope != model.AssetLock && scope != model.PlacementLock {
						return nil, fmt.Errorf("unknown lock scope %s", scope)
					}
					st.Lock = scope
				case "unlock":
					st.Lock = ""
					st.Asset = ""
					st.Placement = ""
				case "review":
					if st.State != model.Draft {
						return nil, fmt.Errorf("review requires draft state")
					}
					st.State = model.Review
				case "draft":
					if st.State == model.Draft {
						return nil, fmt.Errorf("already draft")
					}
					st.State = model.Draft
				case "approve", "override":
					if n.layer.Source == nil {
						return nil, fmt.Errorf("artwork selection requires a leaf layer")
					}
					if op.Action == "approve" && st.State != model.Review {
						return nil, fmt.Errorf("approval requires review state")
					}
					path := op.Artifact
					if !filepath.IsAbs(path) {
						path = filepath.Join(w.Root, path)
					}
					path, e = filepath.EvalSymlinks(path)
					if e != nil {
						return nil, e
					}
					data, e := asset.ReadPNG(path)
					if e != nil {
						return nil, e
					}
					if op.Action == "override" {
						st.Manual = path
						st.Pin = ""
						st.State = model.Draft
					} else {
						st.Pin, e = asset.Put(w.Root, data)
						if e != nil {
							return nil, e
						}
						st.Manual = ""
						st.State = model.Approved
					}
					st.Request, e = requestIdentity(n)
					if e != nil {
						return nil, e
					}
				case "clear-selection":
					st.Pin = ""
					st.Manual = ""
					st.Request = ""
					st.State = model.Draft
				default:
					return nil, fmt.Errorf("unknown editorial action %q", op.Action)
				}
				states[key] = st
			}
			for key, st := range states {
				n, ok := nodes[key]
				if !ok {
					if st.Pin != "" || st.Manual != "" {
						return nil, fmt.Errorf("clear selection before deleting %s", key)
					}
					delete(states, key)
					continue
				}
				if st.Pin != "" || st.Manual != "" {
					if n.layer.Source == nil {
						return nil, fmt.Errorf("clear selection before replacing leaf %s", key)
					}
					id, e := requestIdentity(n)
					if e != nil {
						return nil, e
					}
					st.Stale = id != st.Request
					states[key] = st
				}
			}
			// New locks freeze the final prospective state, independent of operation order.
			for _, op := range r.Operations {
				if op.Action != "lock" {
					continue
				}
				key := targetKey(op.Target)
				st := states[key]
				st.Asset, e = protectedAsset(nodes[key], nodes, states, w.Root)
				if e != nil {
					return nil, e
				}
				st.Placement, e = protectedPlacement(nodes[key], nodes)
				if e != nil {
					return nil, e
				}
				states[key] = st
			}
			return json.Marshal(states)
		}
		snap, e := w.Commit(ctx, workspace.Changeset{ExpectedRevision: r.ExpectedRevision, Edits: r.Edits}, nil)
		if e != nil {
			return e
		}
		result = EditResult{Revision: snap.Revision, Layers: states}
		return nil
	})
	return result, err
}

type layerNode struct {
	layer           *model.Layer
	base, placement string
}

func targetKey(t LayerTarget) string {
	return string(t.Page) + "/" + string(t.Panel) + "/" + string(t.Layer)
}
func hash(v any) string { data, _ := json.Marshal(v); return asset.Digest(data) }
func readStates(data []byte) (map[string]LayerStatus, error) {
	states := map[string]LayerStatus{}
	if len(data) > 0 {
		if e := json.Unmarshal(data, &states); e != nil {
			return nil, fmt.Errorf("invalid editorial state: %w", e)
		}
		if states == nil {
			return nil, fmt.Errorf("invalid null editorial state")
		}
	}
	return states, nil
}
func indexLayers(snap *project.Snapshot) map[string]layerNode {
	result := map[string]layerNode{}
	for pi := range snap.Pages {
		p := &snap.Pages[pi]
		for j := range p.Page.Panels {
			panel := &p.Page.Panels[j]
			var visit func([]model.Layer, string)
			visit = func(layers []model.Layer, parent string) {
				for i := range layers {
					l := &layers[i]
					geometry := *l
					geometry.Source = nil
					geometry.Children = nil
					placement := hash([]any{parent, i, geometry})
					result[targetKey(LayerTarget{p.Page.ID, panel.ID, l.ID})] = layerNode{l, filepath.Dir(p.File), hash([]any{placement, placementTree(*l)})}
					visit(l.Children, placement)
				}
			}
			visit(panel.Layers, hash([]any{p.Page.Canvas, p.Page.Layout, p.Page.Background, j, panel.ID}))
		}
	}
	return result
}
func requestIdentity(n layerNode) (string, error) {
	var inputs []any
	var walk func(model.Layer) error
	walk = func(l model.Layer) error {
		if l.Source != nil {
			inputs = append(inputs, *l.Source)
			for _, ref := range []string{l.Source.Path, l.Source.Font} {
				if ref != "" {
					path := ref
					if !filepath.IsAbs(path) {
						path = filepath.Join(n.base, path)
					}
					data, e := os.ReadFile(path)
					if e != nil {
						return e
					}
					inputs = append(inputs, asset.Digest(data))
				}
			}
		}
		for _, c := range l.Children {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(*n.layer); e != nil {
		return "", e
	}
	return hash(inputs), nil
}
func protectedAsset(n layerNode, nodes map[string]layerNode, states map[string]LayerStatus, root string) (string, error) {
	id, e := requestIdentity(n)
	if e != nil {
		return "", e
	}
	parts := []string{id}
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child, ok := nodes[key]
		if !ok || !containsLayer(n.layer, child.layer) {
			continue
		}
		st := states[key]
		if st.Pin != "" {
			if _, e := asset.Resolve(root, st.Pin); e != nil {
				return "", e
			}
			parts = append(parts, key, st.Pin)
		}
		if st.Manual != "" {
			data, e := asset.ReadPNG(st.Manual)
			if e != nil {
				return "", e
			}
			parts = append(parts, key, st.Manual, asset.Digest(data))
		}
	}
	return hash(parts), nil
}
func protectedPlacement(n layerNode, nodes map[string]layerNode) (string, error) {
	parts := []string{n.placement}
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		node := nodes[key]
		if node.layer.Mask == "" || !containsLayer(n.layer, node.layer) && !containsLayer(node.layer, n.layer) {
			continue
		}
		path := node.layer.Mask
		if !filepath.IsAbs(path) {
			path = filepath.Join(node.base, path)
		}
		data, e := asset.ReadPNG(path)
		if e != nil {
			return "", e
		}
		parts = append(parts, key, asset.Digest(data))
	}
	return hash(parts), nil
}
func checkLocks(old, nodes map[string]layerNode, states map[string]LayerStatus, root string) error {
	for key, st := range states {
		if st.Lock == "" {
			continue
		}
		before, ok := old[key]
		if !ok {
			return fmt.Errorf("locked layer missing: %s", key)
		}
		after, ok := nodes[key]
		if !ok {
			return fmt.Errorf("locked layer cannot be deleted: %s", key)
		}
		if st.Lock == model.PlacementLock || st.Lock == model.AllLock {
			identity, e := protectedPlacement(after, nodes)
			if e != nil {
				return e
			}
			if before.placement != after.placement || identity != st.Placement {
				return fmt.Errorf("placement lock protects %s", key)
			}
		}
		if st.Lock == model.AssetLock || st.Lock == model.AllLock {
			identity, e := protectedAsset(before, old, states, root)
			if e != nil {
				return e
			}
			if identity != st.Asset {
				return fmt.Errorf("asset lock detects source replacement: %s", key)
			}
			prospective, e := protectedAsset(after, nodes, states, root)
			if e != nil {
				return e
			}
			if prospective != st.Asset {
				return fmt.Errorf("asset lock protects %s", key)
			}
		}
	}
	return nil
}

func containsLayer(parent, child *model.Layer) bool {
	if parent == nil {
		return false
	}
	if parent == child {
		return true
	}
	for i := range parent.Children {
		if containsLayer(&parent.Children[i], child) {
			return true
		}
	}
	return false
}
func placementTree(l model.Layer) model.Layer {
	l.Source = nil
	children := make([]model.Layer, len(l.Children))
	for i, c := range l.Children {
		children[i] = placementTree(c)
	}
	l.Children = children
	return l
}

func selectedSnapshot(w *workspace.Session) (map[string]LayerStatus, error) {
	if w.Owner != "" && w.Owner != w.Entry {
		full, e := project.Load(w.Owner)
		if e != nil {
			return nil, e
		}
		owner := *w
		owner.Entry = w.Owner
		owner.Snapshot = full
		if _, e = selectedSnapshot(&owner); e != nil {
			return nil, e
		}
	}
	states, e := readStates(w.State)
	if e != nil {
		return nil, e
	}
	nodes := indexLayers(w.Snapshot)
	visible := map[string]LayerStatus{}
	// Verify all selections before mutating the detached scene or invoking a renderer.
	sources := map[string]model.Source{}
	for key, st := range states {
		n, ok := nodes[key]
		if !ok {
			if st.Lock != "" {
				pageID := strings.SplitN(key, "/", 2)[0]
				for _, p := range w.Snapshot.Pages {
					if string(p.Page.ID) == pageID {
						return nil, fmt.Errorf("locked layer missing: %s", key)
					}
				}
				if w.Entry == w.Owner {
					return nil, fmt.Errorf("locked layer missing: %s", key)
				}
			}
			continue
		}
		if st.Lock == model.AssetLock || st.Lock == model.AllLock {
			id, e := protectedAsset(n, nodes, states, w.Root)
			if e != nil {
				return nil, e
			}
			if id != st.Asset {
				return nil, fmt.Errorf("asset lock detects source replacement: %s", key)
			}
		}
		if st.Lock == model.PlacementLock || st.Lock == model.AllLock {
			identity, e := protectedPlacement(n, nodes)
			if e != nil {
				return nil, e
			}
			if identity != st.Placement {
				return nil, fmt.Errorf("placement lock detects external edit: %s", key)
			}
		}
		if st.Pin != "" || st.Manual != "" {
			if n.layer.Source == nil {
				return nil, fmt.Errorf("selected layer is no longer a leaf: %s", key)
			}
			id, e := requestIdentity(n)
			if e != nil {
				return nil, e
			}
			st.Stale = id != st.Request
			path := st.Manual
			if st.Pin != "" {
				path, e = asset.Resolve(w.Root, st.Pin)
			} else {
				_, e = asset.ReadPNG(path)
			}
			if e != nil {
				return nil, e
			}
			sources[key] = model.Source{Kind: "image", Path: path}
		}
		visible[key] = st
	}
	for key, src := range sources {
		n := nodes[key]
		copy := src
		n.layer.Source = &copy
	}
	return visible, nil
}
