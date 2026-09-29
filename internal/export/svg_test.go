package export

import (
	"bytes"
	"context"
	"encoding/xml"
	"github.com/shairozan/PanelTree/internal/layout"
	"github.com/shairozan/PanelTree/model"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestEditablePanelIDsSurviveReordering(t *testing.T) {
	p := model.Page{ID: "page", Canvas: model.Canvas{Width: 100, Height: 100}, Layout: model.Layout{Type: "row", Children: []model.Layout{{Panel: "a"}, {Panel: "b"}}}, Panels: []model.Panel{{ID: "a"}, {ID: "b"}}}
	ids := func() map[string]string {
		t.Helper()
		s := Stage{Dir: t.TempDir(), Page: p}
		r, err := layout.Resolve(context.Background(), p, layout.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.editableSVG(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, "page.svg"))
		if err != nil {
			t.Fatal(err)
		}
		d := xml.NewDecoder(bytes.NewReader(b))
		result := map[string]string{}
		seen := map[string]bool{}
		for {
			tok, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				t.Fatal(e)
			}
			if el, ok := tok.(xml.StartElement); ok {
				attrs := map[string]string{}
				for _, a := range el.Attr {
					attrs[a.Name.Local] = a.Value
				}
				if id := attrs["id"]; id != "" {
					if seen[id] {
						t.Fatal("duplicate SVG id")
					}
					seen[id] = true
				}
				if attrs["data-kind"] == "panel" {
					result[attrs["data-id"]] = attrs["id"]
				}
			}
		}
		return result
	}
	before := ids()
	p.Layout.Children[0], p.Layout.Children[1] = p.Layout.Children[1], p.Layout.Children[0]
	after := ids()
	if len(before) != 2 || len(after) != 2 {
		t.Fatal("missing authored panel identities")
	}
	for k, id := range before {
		if after[k] != id {
			t.Fatalf("panel %s changed ID after reorder", k)
		}
	}
}
