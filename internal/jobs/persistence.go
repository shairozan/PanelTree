package jobs

import (
	"context"
	"encoding/json"
)

// Persistence keeps durable state below the existing job execution policy.
type Persistence interface {
	Read(string) (Job, error)
	Write(Job) error
	List() ([]Job, error)
	Submit(Job, json.RawMessage) error
	Bytes(string, string) ([]byte, error)
	PutArtifact(string, []byte) error
	Lock(context.Context, string, bool) (func(), error)
}

func NewPersistent(root string, p Persistence) (*Store, error) {
	s, e := Open(root)
	if e != nil {
		return nil, e
	}
	s.persistence = p
	return s, nil
}
