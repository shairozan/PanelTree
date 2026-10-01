// Package storage provides alternate authoring persistence below application policy.
package storage

import (
	"context"
	"github.com/shairozan/PanelTree/internal/workspace"
	"os"
	"path/filepath"
)

type Config struct {
	DSNEnv   string `mapstructure:"dsn-env"`
	BlobRoot string `mapstructure:"blob-root"`
}
type ProjectStore interface {
	Open(context.Context, string, func(*workspace.Session) error) error
	Import(context.Context, string, string) error
	Export(context.Context, string, string) error
}
type Filesystem struct{}

func (Filesystem) Open(ctx context.Context, path string, fn func(*workspace.Session) error) error {
	if info, e := os.Stat(path); e == nil && info.IsDir() {
		path = filepath.Join(path, "project.yaml")
	}
	return workspace.Open(ctx, path, fn)
}
