package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenThroughParentAlias(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(target, "jobs"), 0700); err != nil {
		t.Fatal(err)
	}
	alias := parentAlias(t, base, target)
	s, err := Open(filepath.Join(alias, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(filepath.Join(target, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != real {
		t.Fatalf("store root %q, want canonical %q", s.Root, real)
	}
	j := submit(t, s, "aliased-parent")
	other, err := Open(real)
	if err != nil {
		t.Fatal(err)
	}
	got := submit(t, other, "aliased-parent")
	if got.ID != j.ID {
		t.Fatal("aliases did not share the same durable job")
	}
}

func TestJobOwnerProcess(t *testing.T) {
	root := os.Getenv("PANELTREE_TEST_JOB_OWNER")
	if root == "" {
		t.Skip("subprocess helper")
	}
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Run(context.Background(), 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
		fmt.Println("owner-ready")
		<-ctx.Done()
		return nil, ctx.Err()
	}); e != nil {
		t.Fatal(e)
	}
}
func TestProcessDeathRecovery(t *testing.T) {
	s := store(t)
	j := submit(t, s, "process-owner")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestJobOwnerProcess$")
	cmd.Env = append(os.Environ(), "PANELTREE_TEST_JOB_OWNER="+s.Root)
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() || scanner.Text() != "owner-ready" {
		t.Fatalf("owner did not start: %s %v", scanner.Text(), scanner.Err())
	}
	live, e := s.Get(context.Background(), j.ID)
	if e != nil || live.State != Running {
		t.Fatalf("live owner incorrectly recovered: %+v %v", live, e)
	}
	if e = cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = cmd.Wait()
	recovered, e := s.Get(context.Background(), j.ID)
	if e != nil || recovered.State != Failed || recovered.Diagnostic == nil || recovered.Diagnostic.Code != "interrupted" {
		t.Fatalf("dead owner not recovered: %+v %v", recovered, e)
	}
}

func TestRecoveryAbandonedOwner(t *testing.T) {
	s := store(t)
	j := submit(t, s, "interrupted")
	j.State = Running
	j.Sequence = 2
	j.Progress = 50
	data, e := json.Marshal(j)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.Root, j.ID, "00000002.json"), data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(s.Root, j.ID, "artifact"), []byte("orphan partial result"), 0600); e != nil {
		t.Fatal(e)
	}
	other, e := Open(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := other.Get(context.Background(), j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if recovered.State != Failed || recovered.Diagnostic == nil || recovered.Diagnostic.Code != "interrupted" || recovered.ArtifactHash != "" {
		t.Fatalf("phantom running job: %+v", recovered)
	}
	if _, _, _, e = other.Result(context.Background(), j.ID); e == nil {
		t.Fatal("recovered partial result exposed")
	}
}

func store(t *testing.T) *Store {
	t.Helper()
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func submit(t *testing.T, s *Store, key string) Job {
	t.Helper()
	j, e := s.Submit(context.Background(), key, "revision-1", json.RawMessage(`{"input":"frozen"}`))
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func TestDurableIdempotency(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			j, e := s.Submit(ctx, "same", "rev", json.RawMessage(`{"a":1}`))
			if e != nil {
				t.Error(e)
				return
			}
			ids <- j.ID
		})
	}
	wg.Wait()
	close(ids)
	var id string
	for v := range ids {
		if id != "" && id != v {
			t.Fatal("duplicate job")
		}
		id = v
	}
	if id == "" {
		t.Fatal("no submitted job")
	}
	reopened, e := Open(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	j, e := reopened.Get(ctx, id)
	if e != nil || j.State != Queued || j.Revision != "rev" {
		t.Fatalf("lost queued job: %+v %v", j, e)
	}
	if _, e = s.Submit(ctx, "same", "other", json.RawMessage(`{"a":2}`)); e == nil {
		t.Fatal("idempotency key rebound")
	}
}
func TestBoundedExecutionAndOwnership(t *testing.T) {
	s := store(t)
	for i := range 5 {
		submit(t, s, fmt.Sprint(i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{}, 5)
	release := make(chan struct{})
	var active, max atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, 2, func(ctx context.Context, _ json.RawMessage, progress func(int) error) ([]byte, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				old := max.Load()
				if n <= old || max.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			if e := progress(50); e != nil {
				return nil, e
			}
			select {
			case <-release:
				return []byte("complete"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	other, e := Open(s.Root)
	if e != nil {
		t.Fatal(e)
	}
	if e = other.Run(ctx, 2, func(context.Context, json.RawMessage, func(int) error) ([]byte, error) {
		t.Error("second runner executed")
		return nil, nil
	}); e == nil {
		t.Fatal("two process owners allowed")
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if max.Load() != 2 {
		t.Fatalf("worker bound = %d", max.Load())
	}
	all, e := s.List(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, j := range all {
		if j.State != Succeeded || j.Progress != 100 || j.ArtifactHash == "" {
			t.Fatalf("incomplete job: %+v", j)
		}
	}
}
func TestCancellationAndFailureNeverPublishResults(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	queued := submit(t, s, "queued")
	j, e := s.Cancel(ctx, queued.ID)
	if e != nil || j.State != Cancelled {
		t.Fatalf("queued cancel: %+v %v", j, e)
	}
	run := submit(t, s, "running")
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
			close(started)
			<-ctx.Done()
			return []byte("partial"), nil
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("not running")
	}
	if _, e = s.Cancel(ctx, run.ID); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	j, e = s.Get(ctx, run.ID)
	if e != nil || j.State != Cancelled || j.ArtifactHash != "" {
		t.Fatalf("running cancel published result: %+v %v", j, e)
	}
	bad := submit(t, s, "failure")
	if e = s.Run(ctx, 1, func(context.Context, json.RawMessage, func(int) error) ([]byte, error) {
		return []byte("partial"), errors.New("renderer failure")
	}); e != nil {
		t.Fatal(e)
	}
	j, e = s.Get(ctx, bad.ID)
	if e != nil || j.State != Failed || j.ArtifactHash != "" || j.Diagnostic == nil {
		t.Fatalf("failure published result: %+v %v", j, e)
	}
	if _, _, _, e = s.Result(ctx, bad.ID); e == nil {
		t.Fatal("failed result exposed")
	}
}
