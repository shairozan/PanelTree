package jobs

import (
	"context"
	"testing"
)

func TestRecoverResumableExecution(t *testing.T) {
	s := store(t)
	j := submit(t, s, "remote")
	j.State = Running
	j.Execution = []byte(`{"generation_id":"remote-id"}`)
	j.Resumable = true
	if e := s.save(&j); e != nil {
		t.Fatal(e)
	}
	got, e := s.Get(context.Background(), j.ID)
	if e != nil || got.State != Queued || string(got.Execution) != string(j.Execution) {
		t.Fatalf("remote recovery: %+v %v", got, e)
	}
}

func TestResumeRequiresKnownRemoteExecution(t *testing.T) {
	s := store(t)
	j := submit(t, s, "remote")
	j.State = Failed
	j.Execution = []byte(`{"generation_id":"remote"}`)
	j.Resumable = true
	if e := s.save(&j); e != nil {
		t.Fatal(e)
	}
	got, e := s.Resume(context.Background(), j.ID)
	if e != nil || got.State != Queued {
		t.Fatalf("resume: %+v %v", got, e)
	}
	other := submit(t, s, "unknown")
	other.State = Failed
	if e = s.save(&other); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Resume(context.Background(), other.ID); e == nil {
		t.Fatal("unknown execution resumed")
	}
}
