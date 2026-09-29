// Package asset stores approved artwork independently of disposable render caches.
package asset

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/shairozan/PanelTree/internal/workspace"
	"image/png"
	"os"
	"path/filepath"
)

func ReadPNG(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil {
		_ = f.Close()
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > 32<<20 {
		_ = f.Close()
		return nil, fmt.Errorf("artifact must be a regular PNG at most 32 MiB")
	}
	_ = f.Close()
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	cfg, e := png.DecodeConfig(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	if cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > 4<<20 {
		return nil, fmt.Errorf("artifact dimensions exceed renderer limits")
	}
	if _, e = png.Decode(bytes.NewReader(data)); e != nil {
		return nil, e
	}
	return data, nil
}
func Digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func Put(root string, data []byte) (string, error) {
	id := Digest(data)
	dir, e := workspace.SafePath(root, ".paneltree/assets")
	if e != nil {
		return "", e
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, id+".png")
	if old, e := os.ReadFile(path); e == nil {
		if Digest(old) != id {
			return "", fmt.Errorf("corrupt approval pin %s", id)
		}
		return id, nil
	} else if !os.IsNotExist(e) {
		return "", e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return "", e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return "", e
	}
	return id, f.Close()
}
func Resolve(root, id string) (string, error) {
	if len(id) != 64 {
		return "", fmt.Errorf("invalid approval pin")
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", fmt.Errorf("invalid approval pin")
		}
	}
	path, e := workspace.SafePath(root, ".paneltree/assets/"+id+".png")
	if e != nil {
		return "", e
	}
	data, e := ReadPNG(path)
	if e != nil {
		return "", fmt.Errorf("approval pin %s: %w", id, e)
	}
	if Digest(data) != id {
		return "", fmt.Errorf("corrupt approval pin %s", id)
	}
	return path, nil
}
