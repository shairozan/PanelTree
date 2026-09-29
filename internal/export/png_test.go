package export

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestExportFailuresLeaveNoPartialFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.png")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := PNG(ctx, path, image.NewRGBA(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("ignored cancellation")
	}
	if err := os.WriteFile(path, []byte("manual"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PNG(context.Background(), path, image.NewRGBA(image.Rect(0, 0, 1, 1))); err == nil {
		t.Fatal("replaced manual file")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "manual" {
		t.Fatal("manual file changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary output left behind: %v %v", entries, err)
	}
}
