// Package jobs persists local work independently of authoring revisions.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type State string

const (
	Queued    State = "queued"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Diagnostic) Error() string { return e.Code + ": " + e.Message }

type Job struct {
	ID              string         `json:"id"`
	Key             string         `json:"idempotency_key"`
	Revision        model.Revision `json:"revision"`
	State           State          `json:"state"`
	Progress        int            `json:"progress"`
	Diagnostic      *Diagnostic    `json:"diagnostic,omitempty"`
	CancelRequested bool           `json:"cancel_requested,omitempty"`
	InputHash       string         `json:"input_hash"`
	ArtifactHash    string         `json:"artifact_hash,omitempty"`
	Sequence        int            `json:"sequence"`
}
type Handler func(context.Context, json.RawMessage, func(int) error) ([]byte, error)
type Store struct{ Root string }

func Open(root string) (*Store, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	real, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	if !strings.EqualFold(real, root) {
		return nil, fmt.Errorf("job store must not traverse symlinks")
	}
	return &Store{Root: root}, nil
}
func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func (s *Store) path(id, name string) (string, error) {
	if !validID(id) {
		return "", &Diagnostic{"invalid_job", "expected a job ID"}
	}
	return workspace.SafePath(s.Root, filepath.Join(id, name))
}
func flush(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	return f.Close()
}
func atomicFile(dir, name string, data []byte) error {
	f, e := os.CreateTemp(dir, ".pending-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, e = f.Write(data); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}
func (s *Store) save(j *Job) error {
	dir, e := s.path(j.ID, "")
	if e != nil {
		return e
	}
	j.Sequence++
	data, e := json.Marshal(j)
	if e != nil {
		return e
	}
	return atomicFile(dir, fmt.Sprintf("%08d.json", j.Sequence), data)
}
func (s *Store) read(id string) (Job, error) {
	dir, e := s.path(id, "")
	if e != nil {
		return Job{}, e
	}
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return Job{}, &Diagnostic{"not_found", "job does not exist"}
	}
	if e != nil {
		return Job{}, e
	}
	latest := 0
	var name string
	for _, entry := range entries {
		n := entry.Name()
		if len(n) == 13 && strings.HasSuffix(n, ".json") {
			seq, e := strconv.Atoi(n[:8])
			if e == nil && seq > latest {
				latest = seq
				name = n
			}
		}
	}
	if latest == 0 {
		return Job{}, fmt.Errorf("job has no committed state")
	}
	path, e := s.path(id, name)
	if e != nil {
		return Job{}, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return Job{}, e
	}
	var j Job
	if e = json.Unmarshal(data, &j); e != nil {
		return j, e
	}
	if j.ID != id || j.Sequence != latest {
		return j, fmt.Errorf("corrupt job state")
	}
	return j, nil
}
func (s *Store) list() ([]Job, error) {
	entries, e := os.ReadDir(s.Root)
	if e != nil {
		return nil, e
	}
	var all []Job
	for _, entry := range entries {
		if !validID(entry.Name()) {
			continue
		}
		j, e := s.read(entry.Name())
		if e != nil {
			return nil, e
		}
		all = append(all, j)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}
func (s *Store) lock(ctx context.Context, name string, wait bool) (*os.File, error) {
	path, e := workspace.SafePath(s.Root, name)
	if e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	for {
		if e = ctx.Err(); e != nil {
			_ = f.Close()
			return nil, e
		}
		ok, e := tryLock(f)
		if e != nil {
			_ = f.Close()
			return nil, e
		}
		if ok {
			return f, nil
		}
		if !wait {
			_ = f.Close()
			return nil, &Diagnostic{"runner_busy", "another process owns execution"}
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func release(f *os.File) { unlock(f); _ = f.Close() }
func (s *Store) recover() error {
	all, e := s.list()
	if e != nil {
		return e
	}
	for _, j := range all {
		if j.State != Running {
			continue
		}
		j.State = Failed
		j.ArtifactHash = ""
		j.Diagnostic = &Diagnostic{"interrupted", "execution owner exited; submit a new idempotency key to retry"}
		if j.CancelRequested {
			j.State = Cancelled
			j.Diagnostic = &Diagnostic{"cancelled", "cancelled before execution owner exited"}
		}
		if e = s.save(&j); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) with(ctx context.Context, fn func() error) error {
	f, e := s.lock(ctx, "metadata.lock", true)
	if e != nil {
		return e
	}
	defer release(f)
	owner, e := s.lock(ctx, "executor.lock", false)
	if e == nil {
		defer release(owner)
		if e = s.recover(); e != nil {
			return e
		}
	} else {
		var d *Diagnostic
		if !errors.As(e, &d) || d.Code != "runner_busy" {
			return e
		}
	}
	return fn()
}
func (s *Store) Submit(ctx context.Context, key string, revision model.Revision, input json.RawMessage) (Job, error) {
	if key == "" || len(key) > 256 || revision == "" || len(input) > 128<<20 || !json.Valid(input) {
		return Job{}, &Diagnostic{"invalid_request", "key, revision and bounded JSON input are required"}
	}
	var j Job
	e := s.with(ctx, func() error {
		var e error
		id := digest([]byte(key))
		j, e = s.read(id)
		if e == nil {
			if j.Key != key || j.Revision != revision || j.InputHash != digest(input) {
				return &Diagnostic{"idempotency_conflict", "key already identifies different input or revision"}
			}
			return nil
		}
		var d *Diagnostic
		if !errors.As(e, &d) || d.Code != "not_found" {
			return e
		}
		stage, e := os.MkdirTemp(s.Root, ".submit-")
		if e != nil {
			return e
		}
		defer func() { _ = os.RemoveAll(stage) }()
		j = Job{ID: id, Key: key, Revision: revision, State: Queued, InputHash: digest(input), Sequence: 1}
		data, e := json.Marshal(j)
		if e != nil {
			return e
		}
		if e = flush(filepath.Join(stage, "input.json"), input); e != nil {
			return e
		}
		if e = flush(filepath.Join(stage, "00000001.json"), data); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		return os.Rename(stage, filepath.Join(s.Root, id))
	})
	return j, e
}
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	var j Job
	e := s.with(ctx, func() error { var e error; j, e = s.read(id); return e })
	return j, e
}

// Lookup returns the original input for an idempotent retry, even after completion.
func (s *Store) Lookup(ctx context.Context, key string) (Job, json.RawMessage, error) {
	var j Job
	var input []byte
	e := s.with(ctx, func() error {
		var e error
		j, e = s.read(digest([]byte(key)))
		if e != nil {
			return e
		}
		input, e = s.input(j)
		return e
	})
	return j, input, e
}
func (s *Store) List(ctx context.Context) ([]Job, error) {
	var all []Job
	e := s.with(ctx, func() error { var e error; all, e = s.list(); return e })
	return all, e
}
func (s *Store) Cancel(ctx context.Context, id string) (Job, error) {
	var j Job
	e := s.with(ctx, func() error {
		var e error
		j, e = s.read(id)
		if e != nil {
			return e
		}
		switch j.State {
		case Queued:
			j.State = Cancelled
			j.Diagnostic = &Diagnostic{"cancelled", "cancelled before execution"}
		case Running:
			j.CancelRequested = true
		default:
			return nil
		}
		return s.save(&j)
	})
	return j, e
}
func (s *Store) Run(ctx context.Context, workers int, handler Handler) error {
	if workers < 1 || workers > 8 || handler == nil {
		return &Diagnostic{"invalid_workers", "worker count must be between 1 and 8"}
	}
	owner, e := s.lock(ctx, "executor.lock", false)
	if e != nil {
		return e
	}
	defer release(owner)
	if e = s.with(ctx, s.recover); e != nil {
		return e
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Go(func() {
			for {
				var job Job
				e := s.with(ctx, func() error {
					all, e := s.list()
					if e != nil {
						return e
					}
					for _, j := range all {
						if j.State == Queued {
							j.State = Running
							job = j
							return s.save(&job)
						}
					}
					return nil
				})
				if e != nil {
					errs <- e
					return
				}
				if job.ID == "" {
					return
				}
				if e = s.execute(ctx, job, handler); e != nil {
					errs <- e
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			return e
		}
	}
	return ctx.Err()
}
func (s *Store) Result(ctx context.Context, id string) (Job, json.RawMessage, []byte, error) {
	var j Job
	var input, data []byte
	e := s.with(ctx, func() error {
		var e error
		j, e = s.read(id)
		if e != nil {
			return e
		}
		if j.State != Succeeded {
			return &Diagnostic{"not_succeeded", "job has no successful result"}
		}
		input, e = s.input(j)
		if e != nil {
			return e
		}
		path, e := s.path(id, "artifact")
		if e != nil {
			return e
		}
		data, e = os.ReadFile(path)
		if e != nil {
			return e
		}
		if digest(data) != j.ArtifactHash {
			return &Diagnostic{"corrupt_artifact", "job artifact checksum mismatch"}
		}
		return nil
	})
	return j, input, data, e
}

func (s *Store) input(j Job) ([]byte, error) {
	path, e := s.path(j.ID, "input.json")
	if e != nil {
		return nil, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	if digest(data) != j.InputHash {
		return nil, &Diagnostic{"corrupt_input", "job input checksum mismatch"}
	}
	return data, nil
}
func (s *Store) execute(parent context.Context, j Job, handler Handler) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		timer := time.NewTicker(20 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-timer.C:
				current, e := s.Get(ctx, j.ID)
				if e != nil || current.CancelRequested {
					cancel()
					return
				}
			}
		}
	}()
	progress := func(n int) error {
		if n < 0 || n > 99 {
			return &Diagnostic{"invalid_progress", "running progress must be 0 through 99"}
		}
		return s.with(ctx, func() error {
			current, e := s.read(j.ID)
			if e != nil {
				return e
			}
			if current.CancelRequested {
				return context.Canceled
			}
			if n <= current.Progress {
				return nil
			}
			current.Progress = n
			return s.save(&current)
		})
	}
	input, runErr := s.input(j)
	var output []byte
	if runErr == nil {
		output, runErr = invoke(ctx, handler, input, progress)
	}
	close(stop)
	<-watchDone
	return s.with(context.Background(), func() error {
		current, e := s.read(j.ID)
		if e != nil {
			return e
		}
		current.ArtifactHash = ""
		switch {
		case current.CancelRequested || ctx.Err() != nil:
			current.State = Cancelled
			current.Diagnostic = &Diagnostic{"cancelled", "execution cancelled; no result published"}
		case runErr != nil:
			current.State = Failed
			current.Diagnostic = &Diagnostic{"execution_failed", runErr.Error()}
			var d *Diagnostic
			if errors.As(runErr, &d) {
				current.Diagnostic = d
			}
		case len(output) == 0 || len(output) > 32<<20:
			current.State = Failed
			current.Diagnostic = &Diagnostic{"invalid_artifact", "empty or oversized result"}
		default:
			path, e := s.path(j.ID, "artifact")
			if e != nil {
				return e
			}
			if e = atomicFile(filepath.Dir(path), "artifact", output); e != nil {
				return e
			}
			current.State = Succeeded
			current.Progress = 100
			current.ArtifactHash = digest(output)
		}
		return s.save(&current)
	})
}
func invoke(ctx context.Context, h Handler, input json.RawMessage, p func(int) error) (data []byte, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("renderer panic: %v", v)
		}
	}()
	return h(ctx, input, p)
}
