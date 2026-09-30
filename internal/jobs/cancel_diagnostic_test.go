package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCancellationPreservesBackendCleanupDiagnostic(t *testing.T) {
	s := store(t)
	j := submit(t, s, "cancel-backend")
	e := s.Run(context.Background(), 1, func(ctx context.Context, _ json.RawMessage, _ func(int) error) ([]byte, error) {
		if _, e := s.Cancel(ctx, j.ID); e != nil {
			return nil, e
		}
		return nil, fmt.Errorf("backend cancellation could not be confirmed: %w", context.Canceled)
	})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.Get(context.Background(), j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.State != Cancelled || got.Diagnostic == nil || !strings.Contains(got.Diagnostic.Message, "backend cancellation could not be confirmed") {
		t.Fatalf("cleanup diagnostic lost: %+v", got)
	}
}
