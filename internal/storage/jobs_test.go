package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/internal/project"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresJobCheckpointRecovery(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	p, e := Connect(ctx, dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	id := "jobs-" + filepath.Base(filepath.Dir(root))
	if e = p.Import(ctx, id, source); e != nil {
		t.Fatal(e)
	}
	s, e := p.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.Submit(ctx, "one", "revision", json.RawMessage(`{"prompt":"test"}`))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Run(ctx, 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
		if e := jobs.Checkpoint(ctx, map[string]string{"generation_id": "remote"}, true); e != nil {
			return nil, e
		}
		return nil, fmt.Errorf("interrupted download")
	}); e != nil {
		t.Fatal(e)
	}
	other, e := p.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = other.Resume(ctx, j.ID); e != nil {
		t.Fatal(e)
	}
	if e = other.Run(ctx, 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
		var state map[string]string
		if e := json.Unmarshal(jobs.Execution(ctx), &state); e != nil {
			return nil, e
		}
		if state["generation_id"] != "remote" {
			t.Error("remote execution lost")
		}
		return []byte("original-output"), nil
	}); e != nil {
		t.Fatal(e)
	}
	result, _, data, e := s.Result(ctx, j.ID)
	if e != nil || result.State != jobs.Succeeded || string(data) != "original-output" {
		t.Fatalf("result %v %v", result, e)
	}
	if _, e = s.Submit(ctx, "one", "revision", json.RawMessage(`{"prompt":"different"}`)); e == nil {
		t.Fatal("idempotency conflict lost")
	}
}

func TestImportExportHoldsUnfinishedJobs(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	local, e := jobs.Open(filepath.Join(filepath.Dir(source), ".paneltree/jobs"))
	if e != nil {
		t.Fatal(e)
	}
	job, e := local.Submit(ctx, "unrun", "revision", json.RawMessage(`{"prompt":"not billed"}`))
	if e != nil {
		t.Fatal(e)
	}
	p, e := Connect(ctx, dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	id := "portable-" + filepath.Base(filepath.Dir(root))
	if e = p.Import(ctx, id, source); e != nil {
		t.Fatal(e)
	}
	remote, e := p.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	j, e := remote.Get(ctx, job.ID)
	if e != nil || j.State != jobs.Cancelled {
		t.Fatalf("imported job must be retained and held: %v %v", j, e)
	}
	dest := filepath.Join(root, "export")
	if e = p.Export(ctx, id, dest); e != nil {
		t.Fatal(e)
	}
	restored, e := jobs.Open(filepath.Join(dest, ".paneltree/jobs"))
	if e != nil {
		t.Fatal(e)
	}
	j, e = restored.Get(ctx, job.ID)
	if e != nil || j.State != jobs.Cancelled {
		t.Fatalf("history lost: %v %v", j, e)
	}
}

func TestPortableJobsRejectSymlink(t *testing.T) {
	root := t.TempDir()
	local, e := jobs.Open(filepath.Join(root, ".paneltree/jobs"))
	if e != nil {
		t.Fatal(e)
	}
	j, e := local.Submit(context.Background(), "symlink", "r", json.RawMessage(`{"prompt":"private"}`))
	if e != nil {
		t.Fatal(e)
	}
	input := filepath.Join(root, ".paneltree/jobs", j.ID, "input.json")
	outside := filepath.Join(t.TempDir(), "input.json")
	if e = os.Rename(input, outside); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, input); e != nil {
		t.Skipf("symlink unsupported: %v", e)
	}
	if _, e = captureJobs(root); e == nil {
		t.Fatal("external symlink imported")
	}
}

func TestPostgresExecutorSessionLoss(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("Postgres unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	p, e := Connect(ctx, dsn, filepath.Join(root, "blobs"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	source, e := project.Init(filepath.Join(root, "source"))
	if e != nil {
		t.Fatal(e)
	}
	id := "fencing-" + filepath.Base(filepath.Dir(root))
	if e = p.Import(ctx, id, source); e != nil {
		t.Fatal(e)
	}
	old, e := p.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	newOwner, e := p.Jobs(id)
	if e != nil {
		t.Fatal(e)
	}
	job, e := old.Submit(ctx, "one-paid-call", "revision", json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	checkpoint := make(chan error, 1)
	go func() {
		done <- old.Run(ctx, 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
			close(started)
			select {
			case <-release:
			case <-time.After(15 * time.Second):
				return nil, fmt.Errorf("test release timed out")
			}
			checkpoint <- jobs.Checkpoint(ctx, map[string]string{"generation_id": "stale"}, true)
			return []byte("stale-artifact"), nil
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	unexpected := func(context.Context, json.RawMessage, func(int) error) ([]byte, error) {
		t.Error("unknown paid request submitted again")
		return nil, fmt.Errorf("must not execute")
	}
	if e = newOwner.Run(ctx, 1, unexpected); e == nil {
		t.Error("concurrent executor accepted")
	}
	c, e := p.connect(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = c.Close(context.Background()) }()
	var killed bool
	e = c.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_locks WHERE locktype='advisory' AND classid=((hashtextextended($1,26)>>32)&4294967295)::oid AND objid=(hashtextextended($1,26)&4294967295)::oid AND objsubid=1 AND granted`, id+"/executor.lock").Scan(&killed)
	if e != nil || !killed {
		t.Fatalf("terminate task-owned executor: %v %v", killed, e)
	}
	if e = newOwner.Run(ctx, 1, unexpected); e != nil {
		t.Fatal(e)
	}
	close(release)
	select {
	case err := <-checkpoint:
		if err == nil {
			t.Error("stale checkpoint accepted")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	final, e := newOwner.Get(ctx, job.ID)
	if e != nil {
		t.Fatal(e)
	}
	if final.State != jobs.Failed || final.ArtifactHash != "" || len(final.Execution) > 0 {
		t.Fatalf("stale worker changed recovered job: %+v", final)
	}
}
