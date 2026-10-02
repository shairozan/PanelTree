package web

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/adapters"
	"github.com/shairozan/PanelTree/internal/jobs"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/model"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	root := t.TempDir()
	s, e := New(Config{Roots: []string{root}, StateDir: t.TempDir(), Authority: "127.0.0.1:8910"}, app.NewService())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	w := request(s, "GET", "/api/session", nil, "")
	if w.Code != 200 {
		t.Fatalf("session: %d %s", w.Code, w.Body)
	}
	var session struct {
		Token string `json:"token"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &session); e != nil {
		t.Fatal(e)
	}
	if session.Token == "" {
		t.Fatal("missing CSRF token")
	}
	return s, root, session.Token
}
func request(s *Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://127.0.0.1:8910"+path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:8910")
	r.Header.Set("X-PanelTree-Token", token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestLocalBoundaryAndProjectJourney(t *testing.T) {
	s, root, token := fixture(t)
	body := map[string]any{"backend": "files", "path": filepath.Join(root, "book")}
	if w := request(s, "POST", "/api/projects", body, ""); w.Code != 403 {
		t.Fatalf("CSRF accepted: %d", w.Code)
	}
	hostile := httptest.NewRequest("GET", "http://evil.example/api/session", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, hostile)
	if w.Code != 403 {
		t.Fatal("untrusted Host accepted")
	}
	bad := request(s, "POST", "/api/projects", map[string]any{"backend": "files", "path": filepath.Join(t.TempDir(), "escape")}, token)
	if bad.Code < 400 {
		t.Fatal("outside root accepted")
	}
	w = request(s, "POST", "/api/projects", body, token)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var project struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &project); e != nil {
		t.Fatal(e)
	}
	w = request(s, "GET", "/api/projects/"+project.ID, nil, token)
	if w.Code != 200 {
		t.Fatalf("open: %s", w.Body)
	}
	var view app.Inspection
	if e := json.Unmarshal(w.Body.Bytes(), &view); e != nil {
		t.Fatal(e)
	}
	if len(view.Pages) != 2 {
		t.Fatal("ordered pages missing")
	}
	w = request(s, "POST", "/api/projects/"+project.ID+"/preview", map[string]any{"page": "page-01"}, token)
	if w.Code != 200 {
		t.Fatalf("preview: %s", w.Body)
	}
	var preview struct {
		URL string `json:"url"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &preview); e != nil {
		t.Fatal(e)
	}
	image := request(s, "GET", preview.URL, nil, token)
	if image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("preview not an authorized PNG: %d", image.Code)
	}
	jobs := request(s, "GET", "/api/projects/"+project.ID+"/jobs", nil, token)
	if jobs.Code != 200 || bytes.Contains(jobs.Body.Bytes(), []byte("queued")) {
		t.Fatal("preview queued generation")
	}
}

func openFixture(t *testing.T) (*Server, string, string, *app.Inspection) {
	t.Helper()
	s, root, token := fixture(t)
	w := request(s, "POST", "/api/projects", map[string]any{"backend": "files", "path": filepath.Join(root, "book")}, token)
	var p Project
	if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	w = request(s, "GET", "/api/projects/"+p.ID, nil, token)
	var view app.Inspection
	if e := json.Unmarshal(w.Body.Bytes(), &view); e != nil {
		t.Fatal(e)
	}
	return s, p.ID, token, &view
}
func TestRevisionAwareUndoAndGenerationReview(t *testing.T) {
	s, id, token, view := openFixture(t)
	base := "/api/projects/" + id
	doc := view.Documents[2].Document
	doc.Page.Panels[0].Layers[0].Role = "edited"
	w := request(s, "POST", base+"/edit", map[string]any{"revision": view.Revision, "edits": []app.DocumentEdit{{File: "pages/01.yaml", Document: doc}}}, token)
	if w.Code != 200 {
		t.Fatalf("edit: %s", w.Body)
	}
	var edit app.EditResult
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	w = request(s, "POST", base+"/undo", map[string]any{"revision": edit.Revision}, token)
	if w.Code != 200 {
		t.Fatalf("undo: %s", w.Body)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	w = request(s, "POST", base+"/redo", map[string]any{"revision": edit.Revision}, token)
	if w.Code != 200 {
		t.Fatalf("redo: %s", w.Body)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	target := app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}
	w = request(s, "POST", base+"/prepare", map[string]any{"revision": edit.Revision, "target": target, "renderer": "builtin", "width": 120, "height": 180, "key": "deliberate"}, token)
	if w.Code != 200 {
		t.Fatalf("prepare: %s", w.Body)
	}
	var quote struct {
		Job    app.Job `json:"job"`
		Ticket string  `json:"ticket"`
		Status string  `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &quote)
	if quote.Job.State != "queued" || quote.Ticket == "" || quote.Status == "" {
		t.Fatal("missing explicit quote state")
	}
	if w = request(s, "POST", base+"/run", map[string]any{"job": quote.Job.ID}, token); w.Code != 403 {
		t.Fatal("unconfirmed execution allowed")
	}
	w = request(s, "POST", base+"/run", map[string]any{"job": quote.Job.ID, "ticket": quote.Ticket, "confirm": true}, token)
	if w.Code != 202 {
		t.Fatalf("run: %s", w.Body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		w = request(s, "GET", base+"/jobs", nil, token)
		var jobs []app.Job
		_ = json.Unmarshal(w.Body.Bytes(), &jobs)
		if len(jobs) == 1 && jobs[0].State == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck: %s", w.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	w = request(s, "GET", base, nil, token)
	var unselected app.Inspection
	_ = json.Unmarshal(w.Body.Bytes(), &unselected)
	if unselected.Revision != edit.Revision {
		t.Fatal("candidate auto-selected")
	}
	w = request(s, "POST", base+"/select", map[string]any{"revision": edit.Revision, "job": quote.Job.ID}, token)
	if w.Code != 200 {
		t.Fatalf("select: %s", w.Body)
	}
	w = request(s, "POST", base+"/undo", map[string]any{"revision": model.Revision("stale")}, token)
	if w.Code != 409 {
		t.Fatalf("stale undo accepted: %d", w.Code)
	}
}

func TestOverrideRejectsParentTraversal(t *testing.T) {
	s, id, token, view := openFixture(t)
	w := request(s, "POST", "/api/projects/"+id+"/edit", map[string]any{"revision": view.Revision, "operations": []app.Operation{{Target: app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, Action: "override", Artifact: "../../outside.png"}}}, token)
	if w.Code != 403 {
		t.Fatalf("traversal reached service: %d %s", w.Code, w.Body)
	}
}

func TestArtworkHTTPJourney(t *testing.T) {
	s, id, token, _ := openFixture(t)
	base := "/api/projects/" + id
	var data bytes.Buffer
	if e := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 512, 768))); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8910"+base+"/upload", bytes.NewReader(data.Bytes()))
	r.Header.Set("Origin", "http://127.0.0.1:8910")
	r.Header.Set("X-PanelTree-Token", token)
	r.Header.Set("Content-Type", "image/png")
	r.Header.Set("X-Artwork-License", url.QueryEscape("Own artwork"))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var a app.Artwork
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil {
		t.Fatal(e)
	}
	w = request(s, "GET", base+"/artworks", nil, token)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Own artwork")) {
		t.Fatalf("provenance: %s", w.Body)
	}
	w = request(s, "GET", base+"/media?path="+url.QueryEscape(a.Path), nil, token)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data.Bytes()) {
		t.Fatal("original changed")
	}

	w = request(s, "GET", base+"/media?thumbnail=1&path="+url.QueryEscape(a.Path), nil, token)
	cfg, e := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if e != nil || cfg.Width > 256 || cfg.Height > 256 {
		t.Fatalf("unbounded thumbnail: %+v %v", cfg, e)
	}
	w = request(s, "GET", base+"/media?path=../../secret.png", nil, token)
	if w.Code < 400 {
		t.Fatal("traversal accepted")
	}
	w = request(s, "GET", base+"/media?path=missing.png", nil, token)
	if w.Code != 404 {
		t.Fatalf("missing media: %d", w.Code)
	}
}

func TestReferenceReviewHTTP(t *testing.T) {
	s, id, token, _ := openFixture(t)
	base := "/api/projects/" + id
	call := func(body map[string]any, code int) app.ReferenceSet {
		t.Helper()
		w := request(s, "POST", base+"/references", body, token)
		if w.Code != code {
			t.Fatalf("reference action: %d %s", w.Code, w.Body)
		}
		var set app.ReferenceSet
		_ = json.Unmarshal(w.Body.Bytes(), &set)
		return set
	}
	set := call(map[string]any{"set": "hero", "action": "create"}, 200)
	set = call(map[string]any{"set": "hero", "action": "import", "revision": set.Revision, "slot": "body/front", "path": "assets/hero.png", "license": "CC0", "attribution": "Demo"}, 200)
	candidate := ""
	for key := range set.Candidates {
		candidate = key
	}
	stale := set.Revision
	set = call(map[string]any{"set": "hero", "action": "accept", "revision": set.Revision, "slot": "body/front", "candidate": candidate}, 200)
	if set.Accepted["body/front"] != candidate {
		t.Fatal("approval missing")
	}
	call(map[string]any{"set": "hero", "action": "reject", "revision": stale, "slot": "body/front", "candidate": candidate}, 409)
	call(map[string]any{"set": "hero", "action": "import", "revision": set.Revision, "slot": "body/left", "path": "../../escape.png", "license": "CC0", "attribution": "Demo"}, 403)
}

func TestLibraryHTTP(t *testing.T) {
	dsn := os.Getenv("PANELTREE_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("PostgreSQL not configured")
	}
	ctx := context.Background()
	db, e := storage.Connect(ctx, dsn, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	s, root, token := fixture(t)
	s.service = app.NewService(app.WithStorage(db))
	w := request(s, "POST", "/api/projects", map[string]any{"backend": "files", "path": filepath.Join(root, "library-story")}, token)
	var p Project
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	base := "/api/projects/" + p.ID
	pkg := model.CharacterPackage{Schema: "paneltree/character/v1", ID: "hero", Version: "v1", Description: "Test hero", References: []model.CharacterReference{{ID: "front", Version: "v1", Path: "assets/hero.png", License: "CC0", Attribution: "Demo", Description: "Front"}}}
	w = request(s, "POST", base+"/character", pkg, token)
	if w.Code != 201 {
		t.Fatalf("create character: %d %s", w.Code, w.Body)
	}
	var authored struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &authored)
	id := "web-" + randomID()
	w = request(s, "POST", base+"/library", map[string]any{"action": "publish", "id": id, "version": "v1", "package": authored.Path}, token)
	if w.Code != 200 {
		t.Fatalf("publish: %s", w.Body)
	}
	w = request(s, "GET", "/api/library", nil, token)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatalf("library list: %s", w.Body)
	}
	w = request(s, "GET", "/api/library/detail?id="+id+"&version=v1", nil, token)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Test hero")) {
		t.Fatalf("library details: %s", w.Body)
	}
	w = request(s, "GET", base, nil, token)
	var view app.Inspection
	_ = json.Unmarshal(w.Body.Bytes(), &view)
	w = request(s, "POST", base+"/library", map[string]any{"action": "use", "id": id, "version": "v1", "expected_revision": view.Revision, "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}}, token)
	if w.Code != 200 {
		t.Fatalf("use: %s", w.Body)
	}
	w = request(s, "GET", base, nil, token)
	if !bytes.Contains(w.Body.Bytes(), []byte("characters/library/"+id+"/v1/character.json")) {
		t.Fatal("pinned version missing")
	}
}

type slowRaster struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *slowRaster) Raster(ctx context.Context, src model.Source, base string) (image.Image, error) {
	b.once.Do(func() { close(b.entered); <-b.release })
	return (adapters.Builtin{}).Raster(ctx, src, base)
}
func TestSlowPreviewDoesNotBlockSession(t *testing.T) {
	s, id, token, _ := openFixture(t)
	b := &slowRaster{entered: make(chan struct{}), release: make(chan struct{})}
	s.service = app.NewService(app.WithRasterizer(b))
	done := make(chan struct{})
	go func() {
		defer close(done)
		request(s, "POST", "/api/projects/"+id+"/preview", map[string]any{"page": "page-01"}, token)
	}()
	select {
	case <-b.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("preview did not start")
	}
	defer func() { close(b.release); <-done }()
	response := make(chan int, 1)
	go func() { response <- request(s, "GET", "/api/session", nil, token).Code }()
	select {
	case code := <-response:
		if code != 200 {
			t.Fatal(code)
		}
	case <-time.After(time.Second):
		t.Fatal("session blocked behind preview")
	}
}
func TestLandscapePreview(t *testing.T) {
	s, id, token, view := openFixture(t)
	doc := view.Documents[2].Document
	doc.Page.Canvas = model.Canvas{Width: 900, Height: 600}
	doc.Page.Layout = model.Layout{Panel: "p1"}
	doc.Page.Panels = []model.Panel{{ID: "p1", Layers: []model.Layer{{ID: "hero", Source: &model.Source{Kind: "image", Path: "../assets/hero.png"}}}}}
	w := request(s, "POST", "/api/projects/"+id+"/edit", map[string]any{"revision": view.Revision, "edits": []app.DocumentEdit{{File: "pages/01.yaml", Document: doc}}}, token)
	if w.Code != 200 {
		t.Fatalf("edit: %s", w.Body)
	}
	w = request(s, "POST", "/api/projects/"+id+"/preview", map[string]any{"page": "page-01"}, token)
	if w.Code != 200 {
		t.Fatalf("landscape preview: %s", w.Body)
	}
}
func TestClosedServerRejectsRequests(t *testing.T) {
	s, _, token := fixture(t)
	s.Close()
	if w := request(s, "GET", "/api/session", nil, token); w.Code != 503 {
		t.Fatalf("closed server still accepting: %d", w.Code)
	}
}

func TestExportDimensions(t *testing.T) {
	s, id, token, _ := openFixture(t)
	base := "/api/projects/" + id
	w := request(s, "POST", base+"/export", map[string]any{"page": "page-01", "width": 1200, "height": 1800}, token)
	if w.Code != 200 {
		t.Fatalf("export: %d %s", w.Code, w.Body)
	}
	var out struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	w = request(s, "GET", out.URL, nil, token)
	cfg, e := png.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if e != nil || cfg.Width != 1200 || cfg.Height != 1800 {
		t.Fatalf("export dimensions: %+v %v", cfg, e)
	}
	w = request(s, "POST", base+"/export", map[string]any{"page": "page-01", "width": 100000, "height": 100000}, token)
	if w.Code != 400 {
		t.Fatal("unbounded export accepted")
	}
}
func TestBackgroundStartFailureVisible(t *testing.T) {
	s, id, token, view := openFixture(t)
	base := "/api/projects/" + id
	prepare := func(key string) (string, string) {
		t.Helper()
		w := request(s, "POST", base+"/prepare", map[string]any{"revision": view.Revision, "target": app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}, "renderer": "builtin", "width": 120, "height": 180, "key": key}, token)
		var out struct {
			Job    app.Job `json:"job"`
			Ticket string  `json:"ticket"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || out.Job.ID == "" {
			t.Fatalf("prepare: %s", w.Body)
		}
		return out.Job.ID, out.Ticket
	}
	first, _ := prepare("first")
	second, ticket := prepare("second")
	store, e := jobs.Open(filepath.Join(filepath.Dir(s.projects[id].Handle), ".paneltree/jobs"))
	if e != nil {
		t.Fatal(e)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = store.RunOne(context.Background(), first, func(context.Context, json.RawMessage, func(int) error) ([]byte, error) {
			close(entered)
			<-release
			return []byte("fixture"), nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("external runner did not start")
	}
	defer func() { close(release); <-done }()
	w := request(s, "POST", base+"/run", map[string]any{"job": second, "ticket": ticket, "confirm": true}, token)
	if w.Code != 202 {
		t.Fatal(w.Body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		w = request(s, "GET", base+"/jobs", nil, token)
		if bytes.Contains(w.Body.Bytes(), []byte("start_failed")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("start error hidden: %s", w.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRejectedCandidateRetainsImage(t *testing.T) {
	s, id, token, view := openFixture(t)
	base := "/api/projects/" + id
	doc := view.Documents[2].Document
	doc.Page.Panels[0].Layers[0].Role = "edited"
	w := request(s, "POST", base+"/edit", map[string]any{"revision": view.Revision, "edits": []app.DocumentEdit{{File: "pages/01.yaml", Document: doc}}}, token)
	if w.Code != 200 {
		t.Fatalf("edit: %s", w.Body)
	}
	var edit app.EditResult
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	w = request(s, "POST", base+"/undo", map[string]any{"revision": edit.Revision}, token)
	if w.Code != 200 {
		t.Fatalf("undo: %s", w.Body)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	w = request(s, "POST", base+"/redo", map[string]any{"revision": edit.Revision}, token)
	if w.Code != 200 {
		t.Fatalf("redo: %s", w.Body)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &edit)
	target := app.LayerTarget{Page: "page-01", Panel: "p1", Layer: "hero"}
	w = request(s, "POST", base+"/prepare", map[string]any{"revision": edit.Revision, "target": target, "renderer": "builtin", "width": 120, "height": 180, "key": "deliberate"}, token)
	if w.Code != 200 {
		t.Fatalf("prepare: %s", w.Body)
	}
	var quote struct {
		Job    app.Job `json:"job"`
		Ticket string  `json:"ticket"`
		Status string  `json:"status"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &quote)
	if quote.Job.State != "queued" || quote.Ticket == "" || quote.Status == "" {
		t.Fatal("missing explicit quote state")
	}
	if w = request(s, "POST", base+"/run", map[string]any{"job": quote.Job.ID}, token); w.Code != 403 {
		t.Fatal("unconfirmed execution allowed")
	}
	w = request(s, "POST", base+"/run", map[string]any{"job": quote.Job.ID, "ticket": quote.Ticket, "confirm": true}, token)
	if w.Code != 202 {
		t.Fatalf("run: %s", w.Body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		w = request(s, "GET", base+"/jobs", nil, token)
		var jobs []app.Job
		_ = json.Unmarshal(w.Body.Bytes(), &jobs)
		if len(jobs) == 1 && jobs[0].State == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job stuck: %s", w.Body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	w = request(s, "GET", base, nil, token)
	var unselected app.Inspection
	_ = json.Unmarshal(w.Body.Bytes(), &unselected)
	if unselected.Revision != edit.Revision {
		t.Fatal("candidate auto-selected")
	}

	w = request(s, "POST", base+"/reject", map[string]any{"job": quote.Job.ID}, token)
	if w.Code != 200 {
		t.Fatalf("reject: %d %s", w.Code, w.Body)
	}
	w = request(s, "POST", base+"/select", map[string]any{"revision": edit.Revision, "job": quote.Job.ID}, token)
	if w.Code != 409 {
		t.Fatal("rejected candidate selected")
	}
	w = request(s, "GET", base+"/media?job="+quote.Job.ID, nil, token)
	if w.Code != 200 {
		t.Fatal("rejected original lost")
	}
	w = request(s, "GET", base+"/jobs", nil, token)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"rejected":true`)) {
		t.Fatal("rejection not durable")
	}
}
