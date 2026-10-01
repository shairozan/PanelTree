package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"path/filepath"
	"testing"
)

func TestReferenceCLI(t *testing.T) {
	r, e := app.NewService().Init(context.Background(), app.InitRequest{Directory: filepath.Join(t.TempDir(), "book")})
	if e != nil {
		t.Fatal(e)
	}
	cmd := Command()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"character-reference", r.ProjectFile, "--action", "create", "--set", "hero"})
	if e = cmd.Execute(); e != nil {
		t.Fatal(e)
	}
	var set app.ReferenceSet
	if e = json.Unmarshal(b.Bytes(), &set); e != nil || set.Revision == "" {
		t.Fatalf("missing reference result: %v %s", e, b.String())
	}
}
