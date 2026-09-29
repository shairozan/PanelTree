// Package cache stores canonical recipe mappings and immutable content blobs.
package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Key(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return Hash(b), nil
}

const MaxBlob = 128 << 20

type Store struct {
	Root         string
	beforeRecipe func() error
	mu           sync.Mutex
}
type Entry struct {
	Data                []byte
	ContentHash, Reason string
	Hit                 bool
}
type record struct {
	Recipe string `json:"recipe"`
	Blob   string `json:"blob"`
	Size   int    `json:"size"`
}

func Open(root string) (*Store, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{"blobs", "recipes", "tmp", "quarantine", "staging"} {
		if err = os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}
func valid(key string) bool {
	if len(key) != 64 {
		return false
	}
	b, err := hex.DecodeString(key)
	return err == nil && hex.EncodeToString(b) == key
}
func (s *Store) Get(ctx context.Context, key string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(ctx, key)
}
func (s *Store) get(ctx context.Context, key string) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if !valid(key) {
		return Entry{}, fmt.Errorf("invalid cache recipe key")
	}
	rp := filepath.Join(s.Root, "recipes", key)
	b, err := read(rp, 4096)
	if os.IsNotExist(err) {
		return Entry{Reason: "missing-recipe"}, nil
	}
	if err != nil && !errors.Is(err, errIntegrity) {
		return Entry{}, err
	}
	var rec record
	if err != nil || json.Unmarshal(b, &rec) != nil || rec.Recipe != key || !valid(rec.Blob) || rec.Size < 0 || rec.Size > MaxBlob {
		if e := s.quarantine(rp); e != nil {
			return Entry{}, e
		}
		return Entry{Reason: "corrupt-recipe"}, nil
	}
	bp := filepath.Join(s.Root, "blobs", rec.Blob)
	data, err := read(bp, MaxBlob)
	if err != nil && !os.IsNotExist(err) && !errors.Is(err, errIntegrity) {
		return Entry{}, err
	}
	if err != nil || len(data) != rec.Size || Hash(data) != rec.Blob {
		if e := s.quarantine(bp); e != nil {
			return Entry{}, e
		}
		if e := s.quarantine(rp); e != nil {
			return Entry{}, e
		}
		return Entry{Reason: "corrupt-blob"}, nil
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	return Entry{Data: data, ContentHash: rec.Blob, Hit: true, Reason: "validated-hit"}, nil
}
func (s *Store) Put(ctx context.Context, key string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !valid(key) || len(data) > MaxBlob {
		return "", fmt.Errorf("invalid recipe key or blob size")
	}
	hash := Hash(data)
	existing, err := s.get(ctx, key)
	if err != nil {
		return "", err
	}
	if existing.Hit {
		if existing.ContentHash != hash {
			return "", fmt.Errorf("recipe %s produced different content; change its version, revision or seed", key)
		}
		return hash, nil
	}
	bp := filepath.Join(s.Root, "blobs", hash)
	prior, e := read(bp, MaxBlob)
	if e == nil && Hash(prior) != hash || errors.Is(e, errIntegrity) {
		if e = s.quarantine(bp); e != nil {
			return "", e
		}
	} else if e != nil && !os.IsNotExist(e) {
		return "", e
	}
	if err = s.publish(ctx, bp, data); err != nil {
		return "", err
	}
	if s.beforeRecipe != nil {
		if err = s.beforeRecipe(); err != nil {
			return "", err
		}
	}
	rec, err := json.Marshal(record{Recipe: key, Blob: hash, Size: len(data)})
	if err != nil {
		return "", err
	}
	if err = s.publish(ctx, filepath.Join(s.Root, "recipes", key), rec); err != nil {
		return "", err
	}
	return hash, nil
}
func (s *Store) Invalidate(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(key) {
		return fmt.Errorf("invalid cache key")
	}
	return s.quarantine(filepath.Join(s.Root, "recipes", key))
}

var errIntegrity = errors.New("invalid cache file")

func read(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errIntegrity
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errIntegrity
	}
	return b, nil
}
func (s *Store) quarantine(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Join(s.Root, "quarantine"), filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	name := f.Name()
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(path, name); err != nil {
		_ = os.Remove(name)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}
func (s *Store) publish(ctx context.Context, path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Join(s.Root, "tmp"), "publish-")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Link(f.Name(), path); err != nil {
		if os.IsExist(err) {
			prior, e := read(path, MaxBlob)
			if e == nil && bytes.Equal(prior, data) {
				return nil
			}
			return fmt.Errorf("concurrent immutable publication conflict: %s", filepath.Base(path))
		}
		return err
	}
	return nil
}
