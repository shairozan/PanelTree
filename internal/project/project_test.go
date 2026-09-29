package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pageYAML = `schema: paneltree/v0.1
page:
  id: page-1
  canvas: {width: 1200, height: 1800}
  layout:
    type: row
    children:
      - panel: p1
  panels:
    - id: p1
      layers:
        - id: hero
          role: character
          source: {kind: image, path: hero.png}
          frame: {x: 0, y: 0, width: 1, height: 1}
`

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadStandalonePage(t *testing.T) {
	p := writeFixture(t, t.TempDir(), "page.yaml", pageYAML)
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 {
		t.Fatalf("pages = %d", len(got.Pages))
	}
}

func TestLoadOrderedBook(t *testing.T) {
	dir := t.TempDir()
	book := writeFixture(t, dir, "book.yaml", "schema: paneltree/v0.1\nbook:\n  id: book\n  chapters: [chapters/one.yaml]\n")
	writeFixture(t, dir, "chapters/one.yaml", "schema: paneltree/v0.1\nchapter:\n  id: ch1\n  pages: [../two.yaml, ../one.yaml]\n")
	writeFixture(t, dir, "one.yaml", pageYAML)
	writeFixture(t, dir, "two.yaml", strings.ReplaceAll(pageYAML, "page-1", "page-2"))
	got, err := Load(book)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 2 {
		t.Fatalf("pages = %d", len(got.Pages))
	}
	if got.Pages[0].Page.ID != "page-2" || got.Pages[1].Page.ID != "page-1" {
		t.Fatal("declared page order lost")
	}
}

func TestInvalidProjectsHaveLocations(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"unknown", strings.Replace(pageYAML, "canvas:", "canavs:", 1), "canavs"},
		{"nested unknown", strings.Replace(pageYAML, "width: 1200", "widht: 1200", 1), "widht"},
		{"duplicate key", strings.Replace(pageYAML, "id: page-1", "id: page-1\n  id: again", 1), "duplicate key"},
		{"version", strings.Replace(pageYAML, "v0.1", "v9", 1), "schema"},
		{"duplicate layer", pageYAML + "        - id: hero\n          source: {kind: image, path: other.png}\n", "duplicate"},
		{"missing panel", strings.Replace(pageYAML, "panel: p1", "panel: absent", 1), "absent"},
		{"bad source", strings.Replace(pageYAML, "kind: image", "kind: magic", 1), "source"},
		{"zero canvas", strings.Replace(pageYAML, "width: 1200", "width: 0", 1), "canvas"},
		{"multiple documents", pageYAML + "---\n" + pageYAML, "one YAML document"},
		{"numeric id", strings.Replace(pageYAML, "id: page-1", "id: 123", 1), "string"},
		{"duplicate panel", pageYAML + "    - {id: p1, layers: []}\n", "duplicate panel"},
		{"duplicate placement", strings.Replace(pageYAML, "- panel: p1", "- panel: p1\n      - panel: p1", 1), "duplicate panel placement"},
		{"source and children", pageYAML + "          children:\n            - id: child\n              source: {kind: svg, path: child.svg}\n", "exactly one"},
		{"unknown source field", strings.Replace(pageYAML, "path: hero.png", "path: hero.png, sampler: foo", 1), "sampler"},
		{"null", strings.Replace(pageYAML, "path: hero.png", "path: null", 1), "null"},
		{"alias", strings.Replace(pageYAML, "canvas: {", "canvas: &canvas {", 1), "anchors"},
		{"nonfinite canvas", strings.Replace(pageYAML, "width: 1200", "width: .inf", 1), "canvas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFixture(t, t.TempDir(), "bad.yaml", tc.input)
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "bad.yaml:") {
				t.Fatalf("want located %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestReferenceFailures(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "cycle"}[cycle], func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, "book.yaml", "schema: paneltree/v0.1\nbook:\n  id: book\n  chapters: [chapter.yaml]\n")
			if cycle {
				writeFixture(t, dir, "chapter.yaml", "schema: paneltree/v0.1\nchapter:\n  id: ch\n  pages: [book.yaml]\n")
			}
			_, err := Load(path)
			want := "chapter.yaml"
			if cycle {
				want = "cycle"
			}
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), ".yaml:") {
				t.Fatalf("want located %s, got %v", want, err)
			}
		})
	}
}

func TestInitAndRefuseOverwrite(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	path, err := Init(dest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 2 {
		t.Fatalf("want two example pages, got %d", len(got.Pages))
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dest); err == nil {
		t.Fatal("existing destination must fail")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing project overwritten")
	}
}

func TestReferenceUniqueness(t *testing.T) {
	for _, repeatedFile := range []bool{false, true} {
		dir := t.TempDir()
		second := "two.yaml"
		if repeatedFile {
			second = "one.yaml"
		}
		path := writeFixture(t, dir, "chapter.yaml", "schema: paneltree/v0.1\nchapter:\n  id: ch\n  pages: [one.yaml, "+second+"]\n")
		writeFixture(t, dir, "one.yaml", pageYAML)
		writeFixture(t, dir, "two.yaml", pageYAML)
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), "duplicate document") {
			t.Fatalf("expected duplicate document diagnostic, got %v", err)
		}
	}
}

func TestRevisionIgnoresCommentsAndLocation(t *testing.T) {
	first := writeFixture(t, t.TempDir(), "page.yaml", pageYAML)
	second := writeFixture(t, t.TempDir(), "page.yaml", "# editorial note\n"+pageYAML)
	a, err := Load(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Revision != b.Revision {
		t.Fatal("comment or absolute path changed semantic revision")
	}
	if err := os.WriteFile(second, []byte(strings.Replace(pageYAML, "hero.png", "new.png", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Revision == c.Revision {
		t.Fatal("source edit did not change revision")
	}
}
