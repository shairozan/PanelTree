package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shairozan/PanelTree/app"
	"path/filepath"
)

func (s *server) projectResource(p string) {
	uri := fmt.Sprintf("paneltree://project/%x", sha256.Sum256([]byte(p)))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources[uri] {
		return
	}
	s.resources[uri] = true
	s.sdk.AddResource(&protocol.Resource{URI: uri, Name: filepath.Base(filepath.Dir(p)), MIMEType: "application/json"}, func(ctx context.Context, _ *protocol.ReadResourceRequest) (*protocol.ReadResourceResult, error) {
		var b []byte
		e := s.guard(ctx, func() error {
			path, e := s.project(p, nil)
			if e != nil {
				return e
			}
			v, e := s.service.Inspect(ctx, app.InspectRequest{ProjectFile: path})
			if e != nil {
				return e
			}
			b, e = json.Marshal(v)
			return e
		})
		if e != nil {
			return nil, e
		}
		return &protocol.ReadResourceResult{Contents: []*protocol.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(b)}}}, nil
	})
}
func (s *server) artifact(p string) (string, error) {
	p, e := s.path(p)
	if e != nil {
		return "", e
	}
	b, e := readFile(p, 32<<20)
	if e != nil {
		return "", e
	}
	hash := sha256.Sum256(b)
	uri := fmt.Sprintf("paneltree://artifact/%x/%x", sha256.Sum256([]byte(p)), hash)
	mime := "application/octet-stream"
	switch filepath.Ext(p) {
	case ".png":
		mime = "image/png"
	case ".svg":
		mime = "image/svg+xml"
	case ".json":
		mime = "application/json"
	case ".yaml", ".yml":
		mime = "application/yaml"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources[uri] {
		return uri, nil
	}
	s.resources[uri] = true
	s.sdk.AddResource(&protocol.Resource{URI: uri, Name: filepath.Base(p), MIMEType: mime}, func(ctx context.Context, _ *protocol.ReadResourceRequest) (*protocol.ReadResourceResult, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		path, e := s.path(p)
		if e != nil {
			return nil, e
		}
		data, e := readFile(path, 32<<20)
		if e != nil {
			return nil, e
		}
		if sha256.Sum256(data) != hash {
			return nil, fmt.Errorf("artifact changed; rebuild to register a new resource")
		}
		return &protocol.ReadResourceResult{Contents: []*protocol.ResourceContents{{URI: uri, MIMEType: mime, Blob: data}}}, nil
	})
	return uri, nil
}
