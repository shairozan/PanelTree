package web

import (
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"github.com/shairozan/PanelTree/internal/storage"
	"github.com/shairozan/PanelTree/internal/workspace"
	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
	"net/http"
	"path/filepath"
	"strings"
)

type change struct{ Before, After []app.DocumentEdit }
type history struct {
	Head       model.Revision
	Undo, Redo []change
}
type actionRequest struct {
	Revision   model.Revision     `json:"revision"`
	Edits      []app.DocumentEdit `json:"edits,omitempty"`
	Operations []app.Operation    `json:"operations,omitempty"`
	Target     app.LayerTarget    `json:"target,omitempty"`
	Renderer   string             `json:"renderer,omitempty"`
	Generation *render.Generation `json:"generation,omitempty"`
	Width      int                `json:"width,omitempty"`
	Height     int                `json:"height,omitempty"`
	Key        string             `json:"key,omitempty"`
	Job        string             `json:"job,omitempty"`
	Ticket     string             `json:"ticket,omitempty"`
	Confirm    bool               `json:"confirm,omitempty"`
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, p Project, action string) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var body actionRequest
	if e := decode(w, r, &body); e != nil {
		failure(w, 400, e)
		return
	}
	ctx := r.Context()
	switch action {
	case "edit", "undo", "redo":
		s.edit(w, r, p, action, body)
	case "prepare":
		j, e := s.service.RequestAsset(ctx, app.AssetRequest{ProjectFile: p.Handle, ExpectedRevision: body.Revision, Target: body.Target, IdempotencyKey: body.Key, Renderer: body.Renderer, Generation: body.Generation, Width: body.Width, Height: body.Height})
		if e != nil {
			failure(w, 409, e)
			return
		}
		quote, e := s.service.EstimateJob(ctx, app.JobRequest{ProjectFile: p.Handle, ID: j.ID})
		status := "quoted"
		reason := ""
		if e != nil {
			status = "quote-unavailable"
			reason = e.Error()
		}
		ticket := randomID()
		s.setTicket(p.ID+"/"+j.ID, ticket)
		j.Execution = nil
		reply(w, 200, map[string]any{"job": j, "ticket": ticket, "status": status, "quote": quote, "reason": reason})
	case "run":
		key := p.ID + "/" + body.Job

		if _, e := s.service.Job(ctx, app.JobRequest{ProjectFile: p.Handle, ID: body.Job}); e != nil {
			failure(w, 400, e)
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			failure(w, 503, fmt.Errorf("workspace server is closing"))
			return
		}
		if !body.Confirm || body.Ticket == "" || s.tickets[key] != body.Ticket {
			s.mu.Unlock()
			failure(w, 403, fmt.Errorf("review a quote or quote-unavailable status and explicitly confirm this job"))
			return
		}
		delete(s.tickets, key)
		delete(s.workerErrors, key)
		s.workers.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.workers.Done()
			_, e := s.service.RunJob(s.ctx, app.JobRequest{ProjectFile: p.Handle, ID: body.Job})
			s.mu.Lock()
			defer s.mu.Unlock()
			s.workerErrors[key] = e
		}()
		reply(w, 202, map[string]string{"status": "started"})
	case "reject":
		j, e := s.service.RejectJob(ctx, app.JobRequest{ProjectFile: p.Handle, ID: body.Job})
		if e != nil {
			failure(w, 409, e)
			return
		}
		j.Execution = nil
		reply(w, 200, j)
	case "cancel", "resume", "quote":
		req := app.JobRequest{ProjectFile: p.Handle, ID: body.Job}
		if action == "cancel" {
			j, e := s.service.CancelJob(ctx, req)
			if e != nil {
				failure(w, 400, e)
				return
			}
			j.Execution = nil
			reply(w, 200, j)
			return
		}
		if action == "resume" {
			if _, e := s.service.ResumeJob(ctx, req); e != nil {
				failure(w, 400, e)
				return
			}
		}
		quote, e := s.service.EstimateJob(ctx, req)
		status := "quoted"
		reason := ""
		if e != nil {
			status = "quote-unavailable"
			reason = e.Error()
		}
		ticket := randomID()
		s.setTicket(p.ID+"/"+body.Job, ticket)
		j, e := s.service.Job(ctx, req)
		if e != nil {
			failure(w, 400, e)
			return
		}
		j.Execution = nil
		reply(w, 200, map[string]any{"job": j, "ticket": ticket, "status": status, "quote": quote, "reason": reason})
	case "select":
		result, e := s.service.SelectCandidate(ctx, app.SelectCandidateRequest{ProjectFile: p.Handle, JobID: body.Job, ExpectedRevision: body.Revision})
		if e != nil {
			failure(w, 409, e)
			return
		}
		s.clearHistory(p.ID)
		reply(w, 200, result)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) edit(w http.ResponseWriter, r *http.Request, p Project, action string, body actionRequest) {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	before, e := s.service.Inspect(r.Context(), app.InspectRequest{ProjectFile: p.Handle})
	if e != nil {
		failure(w, 400, e)
		return
	}
	if body.Revision == "" || body.Revision != before.Revision {
		failure(w, 409, fmt.Errorf("project changed; refresh and reapply your edit"))
		return
	}
	h := s.history[p.ID]
	if h == nil {
		h = &history{Head: before.Revision}
		s.history[p.ID] = h
	}
	var step change
	if action != "edit" {
		if h.Head != before.Revision {
			failure(w, 409, fmt.Errorf("history conflicts with a newer edit; refresh before continuing"))
			return
		}
		stack := h.Undo
		if action == "redo" {
			stack = h.Redo
		}
		if len(stack) == 0 {
			failure(w, 409, fmt.Errorf("nothing to %s", action))
			return
		}
		step = stack[len(stack)-1]
		body.Edits = step.Before
		if action == "redo" {
			body.Edits = step.After
		}
		body.Operations = nil
	} else {
		step.After = body.Edits
		for _, edit := range body.Edits {
			found := false
			for _, doc := range before.Documents {
				file := doc.File
				if filepath.IsAbs(file) {
					file, e = filepath.Rel(filepath.Dir(p.Handle), file)
					if e != nil {
						failure(w, 400, e)
						return
					}
				}
				if filepath.ToSlash(file) == edit.File {
					step.Before = append(step.Before, app.DocumentEdit{File: edit.File, Document: doc.Document})
					found = true
					break
				}
			}
			if !found {
				failure(w, 400, fmt.Errorf("edit an existing document"))
				return
			}
		}
	}
	for _, edit := range body.Edits {
		root := filepath.Dir(p.Handle)
		if strings.HasPrefix(p.Handle, "pg:") {
			root = s.cfg.StateDir
		}
		if e = storage.ValidateDocumentPaths(root, edit.File, edit.Document); e != nil {
			failure(w, 403, e)
			return
		}
	}
	// Artwork selections may only reference originals already inside the project.
	for _, op := range body.Operations {
		if op.Artifact != "" {
			root := filepath.Dir(p.Handle)
			if strings.HasPrefix(p.Handle, "pg:") {
				root = s.cfg.StateDir
			}
			if _, e = workspace.SafePath(root, op.Artifact); e != nil {
				failure(w, 403, fmt.Errorf("use an imported project artwork"))
				return
			}
		}
	}
	result, e := s.service.Edit(r.Context(), app.EditRequest{ProjectFile: p.Handle, ExpectedRevision: before.Revision, Edits: body.Edits, Operations: body.Operations})
	if e != nil {
		failure(w, 409, e)
		return
	}
	switch action {
	case "undo":
		h.Undo = h.Undo[:len(h.Undo)-1]
		h.Redo = append(h.Redo, step)
	case "redo":
		h.Redo = h.Redo[:len(h.Redo)-1]
		h.Undo = append(h.Undo, step)
	default:
		if h.Head != before.Revision || len(body.Operations) > 0 {
			h.Undo = nil
		}
		h.Redo = nil
		if len(body.Edits) > 0 && len(body.Operations) == 0 {
			h.Undo = append(h.Undo, step)
			if len(h.Undo) > 50 {
				h.Undo = h.Undo[1:]
			}
		}
	}
	h.Head = result.Revision
	reply(w, 200, result)
}
