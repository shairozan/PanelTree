package jobs

import (
	"context"
	"encoding/json"
	"fmt"
)

type executionKey struct{}
type executionHandle struct {
	store   *Store
	id      string
	initial json.RawMessage
}

func Execution(ctx context.Context) json.RawMessage {
	h, _ := ctx.Value(executionKey{}).(*executionHandle)
	if h == nil {
		return nil
	}
	return append(json.RawMessage(nil), h.initial...)
}

// Checkpoint persists remote submission state before the next network operation.
func Checkpoint(ctx context.Context, state any, resumable bool) error {
	h, _ := ctx.Value(executionKey{}).(*executionHandle)
	if h == nil {
		return fmt.Errorf("missing job execution context")
	}
	data, e := json.Marshal(state)
	if e != nil {
		return e
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("execution checkpoint too large")
	}
	return h.store.with(context.WithoutCancel(ctx), func() error {
		j, e := h.store.read(h.id)
		if e != nil {
			return e
		}
		j.Execution = data
		j.Resumable = resumable
		return h.store.save(&j)
	})
}

func (s *Store) Resume(ctx context.Context, id string) (Job, error) {
	var j Job
	e := s.with(ctx, func() error {
		var e error
		j, e = s.read(id)
		if e != nil {
			return e
		}
		if !j.Resumable || len(j.Execution) == 0 || (j.State != Failed && j.State != Cancelled) {
			return fmt.Errorf("only failed/cancelled jobs with known remote execution can resume")
		}
		j.State = Queued
		j.CancelRequested = false
		j.Diagnostic = nil
		return s.save(&j)
	})
	return j, e
}
