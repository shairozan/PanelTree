package mcp

import (
	"github.com/shairozan/PanelTree/model"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestImplicitFontDependencyBoundary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	font := filepath.Join(root, "font.ttf")
	if b, e := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", font+".LICENSE", outside).CombinedOutput(); e != nil {
		t.Fatalf("junction: %v %s", e, b)
	}
	s := &server{roots: []string{root}}
	doc := model.Document{Page: &model.Page{Panels: []model.Panel{{Layers: []model.Layer{{Source: &model.Source{Kind: "text", Font: font}}}}}}}
	if e := s.documentPaths(root, doc); e == nil {
		t.Fatal("implicit font companion path was not checked against roots")
	}
}
