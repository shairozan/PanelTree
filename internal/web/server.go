// Package web hosts the local browser workspace over shared application services.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Config struct {
	Roots               []string
	StateDir, Authority string
}
type Project struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Backend string `json:"backend"`
	Handle  string `json:"-"`
}
type registration struct {
	Handle string `json:"handle"`
}
type artifact struct{ Path, Kind string }
type Server struct {
	cfg           Config
	service       *app.Service
	token         string
	mu            sync.Mutex
	editMu        sync.Mutex
	registryMu    sync.Mutex
	closed        bool
	artifactOrder []string
	projects      map[string]Project
	artifacts     map[string]artifact
	history       map[string]*history
	tickets       map[string]string
	workerErrors  map[string]error
	ctx           context.Context
	cancel        context.CancelFunc
	workers       sync.WaitGroup
}

func randomID() string {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func projectID(handle string) string {
	h := sha256.Sum256([]byte(handle))
	return hex.EncodeToString(h[:16])
}
func New(cfg Config, service *app.Service) (*Server, error) {
	if service == nil || len(cfg.Roots) == 0 || cfg.StateDir == "" || cfg.Authority == "" {
		return nil, fmt.Errorf("service, project roots, host and state directory required")
	}
	cfg.Roots = append([]string(nil), cfg.Roots...)
	for i, root := range cfg.Roots {
		p, e := filepath.Abs(root)
		if e != nil {
			return nil, e
		}
		p, e = filepath.EvalSymlinks(p)
		if e != nil {
			return nil, e
		}
		st, e := os.Stat(p)
		if e != nil || !st.IsDir() {
			return nil, fmt.Errorf("invalid project root")
		}
		cfg.Roots[i] = p
	}
	state, e := filepath.Abs(cfg.StateDir)
	if e != nil {
		return nil, e
	}
	cfg.StateDir = state
	if e = os.MkdirAll(filepath.Join(state, "artifacts"), 0700); e != nil {
		return nil, e
	}
	s := &Server{cfg: cfg, service: service, token: randomID(), projects: map[string]Project{}, artifacts: map[string]artifact{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.history = map[string]*history{}
	s.tickets = map[string]string{}
	s.workerErrors = map[string]error{}
	b, e := os.ReadFile(filepath.Join(state, "projects.json"))
	if e == nil {
		var all []registration
		if e = json.Unmarshal(b, &all); e != nil {
			return nil, e
		}
		for _, v := range all {
			if _, e = s.filePath(v.Handle); e != nil {
				return nil, e
			}
			s.register(v.Handle)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	return s, nil
}
func (s *Server) register(handle string) Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	backend := "files"
	name := filepath.Base(filepath.Dir(handle))
	if strings.HasPrefix(handle, "pg:") {
		backend = "postgres"
		name = strings.TrimPrefix(handle, "pg:")
	}
	p := Project{ID: projectID(handle), Name: name, Backend: backend, Handle: handle}
	s.projects[p.ID] = p
	return p
}
func (s *Server) saveRegistry() error {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	s.mu.Lock()
	all := []registration{}
	for _, p := range s.projects {
		if p.Backend == "files" {
			all = append(all, registration{p.Handle})
		}
	}
	s.mu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].Handle < all[j].Handle })
	b, e := json.Marshal(all)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.cfg.StateDir, ".registry-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, e = f.Write(b); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(s.cfg.StateDir, "projects.json"))
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, e error) {
	reply(w, status, map[string]string{"error": e.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("JSON content type required")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		return fmt.Errorf("one JSON value required")
	}
	return nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	origin := "http://" + s.cfg.Authority
	if r.Host != s.cfg.Authority || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		failure(w, 403, fmt.Errorf("local origin required"))
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if r.Header.Get("Origin") != origin || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-PanelTree-Token")), []byte(s.token)) != 1 {
			failure(w, 403, fmt.Errorf("invalid origin or CSRF token"))
			return
		}
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		failure(w, 503, fmt.Errorf("workspace server is closing"))
		return
	}
	switch {
	case r.Method == "GET" && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/")):
		s.static(w, r)
	case r.URL.Path == "/api/session" && r.Method == "GET":
		reply(w, 200, map[string]any{"token": s.token, "roots": s.cfg.Roots, "postgres": s.service.HasStorage(), "renderers": s.service.Renderers()})
	case strings.HasPrefix(r.URL.Path, "/api/library") && r.Method == "GET":
		s.library(w, r)
	case r.URL.Path == "/api/projects":
		s.projectList(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/projects/"):
		s.projectRoute(w, r)
	case strings.HasPrefix(r.URL.Path, "/artifacts/") && r.Method == "GET":
		s.mu.Lock()
		a, ok := s.artifacts[strings.TrimPrefix(r.URL.Path, "/artifacts/")]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, e := os.ReadFile(a.Path)
		if e != nil {
			failure(w, 404, e)
			return
		}
		w.Header().Set("Content-Type", a.Kind)
		_, _ = w.Write(b)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) projectList(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		if s.service.HasStorage() {
			result, e := s.service.Storage(r.Context(), app.StorageRequest{Action: "list"})
			if e != nil {
				failure(w, 503, e)
				return
			}
			for _, handle := range result.Projects {
				s.register(handle)
			}
		}
		s.mu.Lock()
		all := []Project{}
		for _, p := range s.projects {
			all = append(all, p)
		}
		s.mu.Unlock()
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		reply(w, 200, all)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var body struct {
		Backend  string `json:"backend"`
		Path     string `json:"path"`
		Register bool   `json:"register"`
	}
	if e := decode(w, r, &body); e != nil {
		failure(w, 400, e)
		return
	}
	var handle string
	switch body.Backend {
	case "postgres":
		if body.Register {
			failure(w, 400, fmt.Errorf("PostgreSQL discovery is automatic"))
			return
		}
		handle = "pg:" + body.Path
	case "files":
		p, e := s.filePath(body.Path)
		if e != nil {
			failure(w, 403, e)
			return
		}
		handle = p
	default:
		failure(w, 400, fmt.Errorf("choose files or postgres"))
		return
	}
	if body.Register {
		if e := s.guardProject(handle); e != nil {
			failure(w, 400, e)
			return
		}
		if _, e := s.service.Inspect(r.Context(), app.InspectRequest{ProjectFile: handle}); e != nil {
			failure(w, 400, e)
			return
		}
	} else {
		result, e := s.service.Init(r.Context(), app.InitRequest{Directory: handle})
		if e != nil {
			failure(w, 400, e)
			return
		}
		handle = result.ProjectFile
	}
	p := s.register(handle)
	if e := s.saveRegistry(); e != nil {
		failure(w, 500, e)
		return
	}
	reply(w, 201, p)
}
func (s *Server) projectRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
	s.mu.Lock()
	p, ok := s.projects[parts[0]]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if e := s.guardProject(p.Handle); e != nil {
		failure(w, 400, e)
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	} else if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	switch {
	case action == "authoring" && r.Method == "POST":
		v, e := s.service.AuthoringAssets(r.Context(), p.Handle)
		if e != nil {
			failure(w, 400, e)
			return
		}
		reply(w, 200, v)
	case (action == "library" || action == "character") && r.Method == "POST":
		s.character(w, r, p, action)
	case action == "references" && r.Method == "POST":
		s.references(w, r, p)
	case action == "upload" || action == "media" || action == "artworks":
		s.media(w, r, p, action)
	case action == "" && r.Method == "GET":
		v, e := s.service.Inspect(r.Context(), app.InspectRequest{ProjectFile: p.Handle})
		if e != nil {
			failure(w, 400, e)
			return
		}
		reply(w, 200, v)
	case action == "jobs" && r.Method == "GET":
		v, e := s.service.Jobs(r.Context(), p.Handle)
		if e != nil {
			failure(w, 400, e)
			return
		}
		for i := range v {
			v[i].Execution = nil
			s.mu.Lock()
			launchErr := s.workerErrors[p.ID+"/"+v[i].ID]
			s.mu.Unlock()
			if launchErr != nil && v[i].State == "queued" {
				v[i].Diagnostic = &app.JobDiagnostic{Code: "start_failed", Message: launchErr.Error()}
			}
		}
		if v == nil {
			v = []app.Job{}
		}
		reply(w, 200, v)
	case (action == "preview" || action == "export") && r.Method == "POST":
		var body struct {
			Page   string `json:"page"`
			Width  int    `json:"width,omitempty"`
			Height int    `json:"height,omitempty"`
		}
		if e := decode(w, r, &body); e != nil {
			failure(w, 400, e)
			return
		}
		id := randomID()
		path := filepath.Join(s.cfg.StateDir, "artifacts", id+".png")
		view, e := s.service.Inspect(r.Context(), app.InspectRequest{ProjectFile: p.Handle})
		if e != nil {
			failure(w, 400, e)
			return
		}
		width, height := 600, 900
		for _, page := range view.Pages {
			if string(page.Page.ID) == body.Page {
				scale := 900 / math.Max(page.Page.Canvas.Width, page.Page.Canvas.Height)
				width = max(1, int(math.Round(page.Page.Canvas.Width*scale)))
				height = max(1, int(math.Round(page.Page.Canvas.Height*scale)))
			}
		}

		if action == "export" {
			width, height = body.Width, body.Height
			if width == 0 && height == 0 {
				for _, page := range view.Pages {
					if string(page.Page.ID) == body.Page {
						width = int(page.Page.Canvas.Width)
						height = int(page.Page.Canvas.Height)
					}
				}
			}
			if width < 1 || height < 1 || width > 8192 || height > 8192 || int64(width)*int64(height) > 4<<20 {
				failure(w, 400, fmt.Errorf("export must be at most 8192 per side and 4 megapixels"))
				return
			}
		}
		result, e := s.service.Build(r.Context(), app.BuildRequest{ProjectFile: p.Handle, PageID: body.Page, Output: path, Width: width, Height: height, Fit: "contain"})
		if e != nil {
			failure(w, 400, e)
			return
		}
		s.mu.Lock()
		s.artifacts[id] = artifact{path, "image/png"}
		s.artifactOrder = append(s.artifactOrder, id)
		if len(s.artifactOrder) > 128 {
			old := s.artifactOrder[0]
			s.artifactOrder = s.artifactOrder[1:]
			_ = os.Remove(s.artifacts[old].Path)
			delete(s.artifacts, old)
		}
		s.mu.Unlock()
		reply(w, 200, map[string]any{"url": "/artifacts/" + id, "revision": result.Revision})
	default:
		s.action(w, r, p, action)
	}
}

func (s *Server) Close() { s.mu.Lock(); s.closed = true; s.cancel(); s.mu.Unlock(); s.workers.Wait() }
func (s *Server) clearHistory(id string) {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	delete(s.history, id)
}
func (s *Server) setTicket(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickets[key] = value
}
