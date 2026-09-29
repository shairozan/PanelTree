//go:build !windows

package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func parentAlias(t *testing.T, base, target string) string {
	t.Helper()
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	return alias
}
