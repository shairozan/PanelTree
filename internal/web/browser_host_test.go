package web

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/render"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type browserProvider struct {
	mu            sync.Mutex
	posts, quotes int
	polls         map[string]int
	image         []byte
}

func (p *browserProvider) RoundTrip(r *http.Request) (*http.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	code := 200
	var body []byte
	switch {
	case r.URL.Host == "api.ideogram.ai" && r.Method == "POST":
		if e := r.ParseMultipartForm(32 << 20); e != nil {
			return nil, e
		}
		if r.MultipartForm != nil {
			defer func() { _ = r.MultipartForm.RemoveAll() }()
		}
		if r.URL.Query().Get("dry_run") == "true" {
			p.quotes++
			body = []byte(`{"estimated_cost_usd":"0.00","fixture":true}`)
		} else {
			p.posts++
			mode := "ok"
			if strings.Contains(r.FormValue("prompt"), "FAIL") {
				mode = "fail"
			}
			if strings.Contains(r.FormValue("prompt"), "SLOW") {
				mode = "slow"
			}
			body = []byte(fmt.Sprintf(`{"generation_id":"%s-%d"}`, mode, p.posts))
		}
	case r.URL.Host == "api.ideogram.ai" && strings.HasPrefix(r.URL.Path, "/v2/generations/"):
		id := strings.TrimPrefix(r.URL.Path, "/v2/generations/")
		p.polls[id]++
		status := "completed"
		if strings.HasPrefix(id, "fail-") {
			status = "failed"
		}
		if strings.HasPrefix(id, "slow-") && p.polls[id] < 8 {
			status = "pending"
		}
		body = []byte(fmt.Sprintf(`{"generation_id":%q,"status":%q,"data":[{"is_image_safe":true,"url":"https://test.ideogram.ai/fixture.png"}]}`, id, status))
	case r.URL.Host == "test.ideogram.ai":
		body = p.image
	default:
		return nil, fmt.Errorf("browser fixture blocked outbound request")
	}
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
}

// TestBrowserHost is an opt-in isolated host. Its transport never contacts a provider.
func TestBrowserHost(t *testing.T) {
	if os.Getenv("PANELTREE_WEB_TEST") != "1" {
		t.Skip("browser fixture host")
	}
	var encoded bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 64, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 64; x++ {
			im.Set(x, y, color.RGBA{R: 177, G: 114, B: 69, A: 255})
		}
	}
	if e := png.Encode(&encoded, im); e != nil {
		t.Fatal(e)
	}
	provider := &browserProvider{polls: map[string]int{}, image: encoded.Bytes()}
	prior := http.DefaultTransport
	http.DefaultTransport = provider
	t.Cleanup(func() { http.DefaultTransport = prior })
	t.Setenv("PANELTREE_FIXTURE_KEY", "test-only-no-provider-access")
	cfg := render.GenerationConfig{Profiles: map[string]render.GenerationProfile{"fixture-edit": {Renderer: "ideogram", Model: "ideogram-4-5", Operation: "edit", MagicPrompt: "off", Quality: "high", Size: "source"}}}
	cfg.Providers.Ideogram.APIKeyEnv = "PANELTREE_FIXTURE_KEY"
	service, e := app.NewRuntimeService("", "", cfg)
	if e != nil {
		t.Fatal(e)
	}
	if dsn := os.Getenv("PANELTREE_TEST_POSTGRES"); dsn != "" {
		ctx := context.Background()
		c, e := pgx.Connect(ctx, dsn)
		if e != nil {
			t.Fatal(e)
		}
		schema := fmt.Sprintf("web_%d", time.Now().UnixNano())
		if _, e = c.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
			t.Fatal(e)
		}
		_ = c.Close(ctx)
		u, e := url.Parse(dsn)
		if e != nil {
			t.Fatal(e)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		db, e := storage.Connect(ctx, u.String(), t.TempDir())
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
		// Configuration is invocation-local; the test schema isolates prior fixture data.
		t.Setenv("PANELTREE_WEB_FIXTURE_DSN", u.String())
		if e = service.ConfigureStorage(ctx, storage.Config{DSNEnv: "PANELTREE_WEB_FIXTURE_DSN", BlobRoot: t.TempDir()}); e != nil {
			t.Fatal(e)
		}
	}
	s, e := New(Config{Roots: []string{t.TempDir()}, StateDir: t.TempDir(), Authority: "127.0.0.1:8910"}, service)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	l, e := net.Listen("tcp", ":8910")
	if e != nil {
		t.Fatal(e)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__test/stats" {
			provider.mu.Lock()
			defer provider.mu.Unlock()
			reply(w, 200, map[string]int{"posts": provider.posts, "quotes": provider.quotes})
			return
		}
		s.ServeHTTP(w, r)
	})
	t.Log("browser host ready")
	server := http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	if e = server.Serve(l); e != nil {
		t.Fatal(e)
	}
}
