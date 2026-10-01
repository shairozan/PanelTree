// Package characters resolves backend-neutral character dependencies.
package characters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var color = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func read(ctx context.Context, root, path string, limit int64) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	p, e := workspace.SafePath(root, path)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	defer func() { _ = f.Close() }()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("character dependency must be a regular file")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("character dependency exceeds size limit")
	}
	return b, ctx.Err()
}

func Resolve(ctx context.Context, root string, use *model.CharacterUse) (*model.ResolvedCharacter, error) {
	if use == nil {
		return nil, nil
	}
	data, e := read(ctx, root, use.Package, 1<<20)
	if e != nil {
		return nil, e
	}
	var tree yaml.Node
	if e = yaml.Unmarshal(data, &tree); e != nil {
		return nil, e
	}
	var check func(*yaml.Node, reflect.Type, int) error
	check = func(n *yaml.Node, typ reflect.Type, depth int) error {
		if depth > 32 || n.Kind == yaml.AliasNode || n.Anchor != "" || n.Tag == "!!null" {
			return fmt.Errorf("character package forbids aliases, anchors, nulls and excessive nesting")
		}
		if n.Kind == yaml.DocumentNode {
			for _, child := range n.Content {
				if e := check(child, typ, depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		if typ.Kind() == reflect.Struct {
			if n.Kind != yaml.MappingNode {
				return fmt.Errorf("character package requires an object")
			}
			fields := map[string]reflect.Type{}
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				fields[strings.Split(f.Tag.Get("json"), ",")[0]] = f.Type
			}
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				key := n.Content[i].Value
				if seen[key] {
					return fmt.Errorf("duplicate character package key %q", key)
				}
				seen[key] = true
				fieldType, ok := fields[key]
				if !ok {
					return fmt.Errorf("unknown character package field %q", key)
				}
				if e := check(n.Content[i+1], fieldType, depth+1); e != nil {
					return e
				}
			}
		} else if typ.Kind() == reflect.Slice {
			if n.Kind != yaml.SequenceNode {
				return fmt.Errorf("character package requires an array")
			}
			for _, child := range n.Content {
				if e := check(child, typ.Elem(), depth+1); e != nil {
					return e
				}
			}
		} else if n.Kind != yaml.ScalarNode {
			return fmt.Errorf("character package requires a scalar")
		}
		return nil
	}
	if e = check(&tree, reflect.TypeFor[model.CharacterPackage](), 0); e != nil {
		return nil, e
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p model.CharacterPackage
	if e = dec.Decode(&p); e != nil {
		return nil, e
	}
	if e = dec.Decode(new(any)); e != io.EOF {
		return nil, fmt.Errorf("character package requires one document")
	}
	if p.Schema != "paneltree/character/v1" || !identifier.MatchString(p.ID) || strings.TrimSpace(p.Version) == "" || strings.TrimSpace(p.Description) == "" {
		return nil, fmt.Errorf("character package requires schema, id, version and description")
	}
	for _, c := range p.Palette {
		if !color.MatchString(c) {
			return nil, fmt.Errorf("invalid character palette color %q", c)
		}
	}
	out := &model.ResolvedCharacter{Use: *use, ID: p.ID, Version: p.Version, Description: p.Description, Palette: p.Palette}
	out.Use.Props = append([]string(nil), use.Props...)
	groups := [][]model.CharacterState{p.Costumes, p.Expressions, p.Poses}
	selected := []string{use.Costume, use.Expression, use.Pose}
	destinations := []**model.CharacterState{&out.Costume, &out.Expression, &out.Pose}
	refs := append([]model.CharacterReference(nil), p.References...)
	for i, group := range groups {
		seen := map[string]bool{}
		for _, state := range group {
			if !identifier.MatchString(state.ID) || seen[state.ID] || strings.TrimSpace(state.Version) == "" || strings.TrimSpace(state.Description) == "" {
				return nil, fmt.Errorf("invalid or duplicate character state %q", state.ID)
			}
			seen[state.ID] = true
			if state.ID == selected[i] {
				copy := state
				*destinations[i] = &copy
				refs = append(refs, state.References...)
			}
		}
		if selected[i] != "" && !seen[selected[i]] {
			return nil, fmt.Errorf("unknown character state %q", selected[i])
		}
	}
	resolve := func(ref model.CharacterReference) (model.ResolvedReference, error) {
		if !identifier.MatchString(ref.ID) || strings.TrimSpace(ref.Version) == "" || strings.TrimSpace(ref.License) == "" || strings.TrimSpace(ref.Attribution) == "" || strings.TrimSpace(ref.Description) == "" {
			return model.ResolvedReference{}, fmt.Errorf("reference requires id, version, license, attribution and description")
		}
		b, e := read(ctx, root, ref.Path, 32<<20)
		if e != nil {
			return model.ResolvedReference{}, e
		}
		return model.ResolvedReference{CharacterReference: ref, SHA256: fmt.Sprintf("%x", sha256.Sum256(b))}, nil
	}
	seenRefs := map[string]bool{}
	for _, ref := range refs {
		if seenRefs[ref.ID] {
			return nil, fmt.Errorf("duplicate reference %q", ref.ID)
		}
		seenRefs[ref.ID] = true
		r, e := resolve(ref)
		if e != nil {
			return nil, e
		}
		out.References = append(out.References, r)
	}
	props := map[string]model.CharacterReference{}
	for _, ref := range p.Props {
		if _, ok := props[ref.ID]; ok {
			return nil, fmt.Errorf("duplicate prop %q", ref.ID)
		}
		props[ref.ID] = ref
	}
	selectedProps := map[string]bool{}
	for _, id := range use.Props {
		ref, ok := props[id]
		if !ok || selectedProps[id] {
			return nil, fmt.Errorf("unknown or duplicate prop %q", id)
		}
		selectedProps[id] = true
		r, e := resolve(ref)
		if e != nil {
			return nil, e
		}
		out.Props = append(out.Props, r)
	}
	return out, nil
}
