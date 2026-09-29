package jobs

import (
	"golang.org/x/sys/windows"
	"testing"
)

func parentAlias(t *testing.T, _, target string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n >= uint32(len(buf)) {
		t.Fatalf("short path: %v (length %d)", err, n)
	}
	alias := windows.UTF16ToString(buf[:n])
	if alias == target {
		t.Skip("filesystem has no short path alias")
	}
	return alias
}
