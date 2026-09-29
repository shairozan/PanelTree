package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCanonicalKeysAndImmutableContent(t *testing.T) {
	a, _ := Key(map[string]any{"a": 1, "b": 2})
	b, _ := Key(map[string]any{"b": 2, "a": 1})
	if a != b {
		t.Fatal("map order changes recipe")
	}
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := s.Put(context.Background(), a, []byte("pixels"))
	if err != nil {
		t.Fatal(err)
	}
	if hash == a || hash != Hash([]byte("pixels")) {
		t.Fatal("recipe key and output hash confused")
	}
	e, err := s.Get(context.Background(), a)
	if err != nil || !e.Hit || string(e.Data) != "pixels" {
		t.Fatalf("cache miss: %+v %v", e, err)
	}
	if _, err = s.Put(context.Background(), a, []byte("different output")); err == nil {
		t.Fatal("immutable recipe was replaced")
	}
}
func TestCorruptionAndInterruptedPublication(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := Hash([]byte("recipe"))
	data := []byte("pixels")
	hash, err := s.Put(context.Background(), key, data)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.Root, "blobs", hash), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := s.Get(context.Background(), key)
	if err != nil || e.Hit || e.Reason != "corrupt-blob" {
		t.Fatalf("corruption reused: %+v %v", e, err)
	}
	if _, err = s.Put(context.Background(), key, data); err != nil {
		t.Fatal(err)
	}
	e, err = s.Get(context.Background(), key)
	if err != nil || !e.Hit {
		t.Fatal("corrupt cache did not recover")
	}
	next := Hash([]byte("next"))
	sentinel := errors.New("interrupted")
	s.beforeRecipe = func() error { return sentinel }
	if _, err = s.Put(context.Background(), next, []byte("orphan")); !errors.Is(err, sentinel) {
		t.Fatalf("interruption: %v", err)
	}
	s.beforeRecipe = nil
	e, err = s.Get(context.Background(), next)
	if err != nil || e.Hit {
		t.Fatal("partial publication became visible")
	}
	if _, err = s.Put(context.Background(), next, []byte("orphan")); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentPublicationAndCancellation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Put(context.Background(), Hash([]byte("same")), []byte("data")); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	key := Hash([]byte("cancelled"))
	if _, err = s.Put(ctx, key, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err = s.Get(context.Background(), "../../outside"); err == nil {
		t.Fatal("unsafe recipe key accepted")
	}
}
