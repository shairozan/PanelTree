package project

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed template
var template embed.FS

// Init creates a new directory only. Existing directories and contents are never changed.
func Init(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.Mkdir(path, 0755); err != nil {
		return "", fmt.Errorf("create new project directory: %w", err)
	}
	err = fs.WalkDir(template, "template", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel("template", filepath.FromSlash(name))
		if err != nil {
			return err
		}
		dest := filepath.Join(path, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		data, err := template.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0644)
	})
	if err != nil {
		return "", fmt.Errorf("initialization incomplete in %s (retained for inspection): %w", path, err)
	}
	return filepath.Join(path, "project.yaml"), nil
}

// DefaultFontFiles returns the bundled font and its redistribution license.
func DefaultFontFiles() ([]byte, []byte, error) {
	font, err := template.ReadFile("template/assets/Go-Regular.ttf")
	if err != nil {
		return nil, nil, err
	}
	license, err := template.ReadFile("template/assets/Go-Regular.ttf.LICENSE")
	return font, license, err
}
