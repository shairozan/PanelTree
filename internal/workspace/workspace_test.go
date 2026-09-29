package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/shairozan/PanelTree/internal/project"
	"github.com/shairozan/PanelTree/model"
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	p, e := project.Init(filepath.Join(t.TempDir(), "book"))
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestRecoveryFinishesInterruptedPublication(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmtBool(partial), func(t *testing.T) {
			p := fixture(t)
			root := filepath.Dir(p)
			ctx := context.Background()
			if e := Open(ctx, p, func(*Session) error { return nil }); e != nil {
				t.Fatal(e)
			}
			snap, e := project.Load(p)
			if e != nil {
				t.Fatal(e)
			}
			book := snap.Documents[0].Document
			book.Book.Title = "recovered"
			page := snap.Documents[2].Document
			page.Page.Background = "#123456"
			a, e := yaml.Marshal(book)
			if e != nil {
				t.Fatal(e)
			}
			b, e := yaml.Marshal(page)
			if e != nil {
				t.Fatal(e)
			}
			data, e := json.Marshal(journal{Files: map[string][]byte{"project.yaml": a, "pages/01.yaml": b, ".paneltree/state.json": []byte("{}")}})
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(root, ".paneltree", "pending.json"), data, 0600); e != nil {
				t.Fatal(e)
			}
			if partial {
				if e = os.WriteFile(p, []byte("partial"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if e = Open(ctx, p, func(s *Session) error {
				if s.Snapshot.Documents[0].Document.Book.Title != "recovered" || s.Snapshot.Pages[0].Page.Background != "#123456" || string(s.State) != "{}" {
					t.Fatal("mixed revision after recovery")
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			if _, e = os.Stat(filepath.Join(root, ".paneltree", "pending.json")); !os.IsNotExist(e) {
				t.Fatalf("journal not retired: %v", e)
			}
		})
	}
}
func fmtBool(v bool) string {
	if v {
		return "during-publication"
	}
	return "before-publication"
}

func TestEditDocumentWithYMLExtension(t *testing.T) {
	p := fixture(t)
	root := filepath.Dir(p)
	old := filepath.Join(root, "pages", "01.yaml")
	next := filepath.Join(root, "pages", "01.yml")
	if e := os.Rename(old, next); e != nil {
		t.Fatal(e)
	}
	chapter := filepath.Join(root, "chapters", "01.yaml")
	data, e := os.ReadFile(chapter)
	if e != nil {
		t.Fatal(e)
	}
	data = []byte(strings.ReplaceAll(string(data), "01.yaml", "01.yml"))
	if e = os.WriteFile(chapter, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = Open(context.Background(), p, func(s *Session) error {
		d := s.Snapshot.Documents[2].Document
		d.Page.Background = "#123456"
		_, e := s.Commit(context.Background(), Changeset{s.Snapshot.Revision, []Edit{{"pages/01.yml", d}}}, nil)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e = Open(context.Background(), p, func(*Session) error { return nil }); e != nil {
		t.Fatal(e)
	}
}
func TestLockWaitCancellation(t *testing.T) {
	p := fixture(t)
	e := Open(context.Background(), p, func(*Session) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		err := Open(ctx, p, func(*Session) error { t.Fatal("concurrent lock acquired"); return nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting lock ignored cancellation: %v", err)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestCancelledCommitLeavesOldRevision(t *testing.T) {
	p := fixture(t)
	e := Open(context.Background(), p, func(s *Session) error {
		before := s.Snapshot.Revision
		d := s.Snapshot.Documents[0].Document
		d.Book.Title = "cancelled"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := s.Commit(ctx, Changeset{before, []Edit{{"project.yaml", d}}}, nil)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled edit: %v", err)
		}
		snap, e := project.Load(p)
		if e != nil {
			return e
		}
		if snap.Revision != before {
			t.Fatal("cancelled changes published")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestRevisionConflict(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	var c Changeset
	if err := Open(ctx, p, func(s *Session) error {
		c.ExpectedRevision = s.Snapshot.Revision
		d := s.Snapshot.Documents[0].Document
		d.Book.Title = "edited"
		c.Edits = []Edit{{"project.yaml", d}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { results <- Open(ctx, p, func(s *Session) error { _, e := s.Commit(ctx, c, nil); return e }) })
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if strings.Contains(e.Error(), "revision conflict") {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflict)
	}
	snap, e := project.Load(p)
	if e != nil || snap.Documents[0].Document.Book.Title != "edited" {
		t.Fatalf("lost edit: %v", e)
	}
}
func TestValidateEntireChangeset(t *testing.T) {
	p := fixture(t)
	before, _ := os.ReadFile(p)
	err := Open(context.Background(), p, func(s *Session) error {
		d := s.Snapshot.Documents[0].Document
		d.Book.Title = "must not publish"
		bad := model.Document{Schema: model.Schema, Page: &model.Page{ID: "bad"}}
		_, e := s.Commit(context.Background(), Changeset{s.Snapshot.Revision, []Edit{{"project.yaml", d}, {"pages/01.yaml", bad}}}, nil)
		return e
	})
	if err == nil {
		t.Fatal("invalid multi-file edit accepted")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("partially published invalid changeset")
	}
	// A successful no-op open distinguishes validation from an unavailable workspace.
	if e := Open(context.Background(), p, func(*Session) error { return nil }); e != nil {
		t.Fatal(e)
	}
}
