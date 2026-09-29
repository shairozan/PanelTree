package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectProjectServices(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	created, err := s.Init(ctx, InitRequest{Directory: filepath.Join(t.TempDir(), "book")})
	if err != nil {
		t.Fatal(err)
	}
	request := InspectRequest{ProjectFile: created.ProjectFile}
	result, err := s.Validate(ctx, request)
	if err != nil || !result.Valid || result.PageCount != 2 {
		t.Fatalf("validate: %+v %v", result, err)
	}
	view, err := s.Inspect(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if view.Pages[0].Page.ID != "page-01" || view.Pages[1].Page.ID != "page-02" {
		t.Fatal("incorrect page order")
	}
}

func TestCancelledInitDoesNotWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "book")
	_, err := NewService().Init(ctx, InitRequest{Directory: path})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled init wrote directory: %v", err)
	}
}
