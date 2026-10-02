package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shairozan/PanelTree/model"
	"github.com/shairozan/PanelTree/render"
)

const characterFixtureJSON = `{"schema":"paneltree/character/v1","id":"alex","version":"1","description":"Alex, short black hair, brown eyes","palette":["#ef6848"],"references":[{"id":"front","version":"1","path":"assets/hero.png","license":"CC0-1.0","attribution":"PanelTree demo","description":"front view"}],"costumes":[{"id":"coat","version":"1","description":"orange coat"},{"id":"summer","version":"1","description":"white shirt"}],"expressions":[{"id":"calm","version":"1","description":"calm face"}],"poses":[{"id":"standing","version":"1","description":"standing upright"}],"props":[{"id":"lantern","version":"1","path":"assets/lantern.svg","license":"CC0-1.0","attribution":"PanelTree demo","description":"rectangular brass lantern"}]}`

func characterFixture(t *testing.T) (*Service, string, *Inspection) {
	t.Helper()
	_, p, _ := editFixture(t)
	file := filepath.Join(filepath.Dir(p), "pages", "01.yaml")
	data, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	data = []byte(strings.ReplaceAll(string(data), "path: ../assets/hero.png}", "path: ../assets/hero.png, character: {package: characters/alex.json, costume: coat, expression: calm, pose: standing, props: [lantern]}}"))
	if e = os.WriteFile(file, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(filepath.Join(filepath.Dir(p), "characters"), 0700); e != nil {
		t.Fatal(e)
	}
	writeCharacter(t, p, characterFixtureJSON)
	c, _ := generationFixture(t)
	s := NewService(WithComfyUI(c))
	i, e := s.Inspect(context.Background(), InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	return s, p, i
}
func writeCharacter(t *testing.T, p, body string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(filepath.Dir(p), "characters", "alex.json"), []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
}
func characterRequest(p string, rev model.Revision, key string, target LayerTarget) AssetRequest {
	return AssetRequest{ProjectFile: p, ExpectedRevision: rev, IdempotencyKey: key, Target: target, Renderer: "comfyui", Generation: &render.Generation{Prompt: "comic portrait", Seed: 17}, Width: 120, Height: 180}
}
func TestCharacterSharedPackageAndProvenance(t *testing.T) {
	s, p, i := characterFixture(t)
	for n, target := range []LayerTarget{target(), {"page-01", "p2", "closeup"}} {
		j, e := s.RequestAsset(context.Background(), characterRequest(p, i.Revision, []string{"first", "second"}[n], target))
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(j)
		for _, want := range []string{"Alex, short black hair", "orange coat", "CC0-1.0", "reference-images", "exact-props"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("job provenance missing %q: %s", want, b)
			}
		}
	}
}
func TestCharacterSelectiveInvalidation(t *testing.T) {
	for _, change := range []string{"selected-costume", "reference-path", "reference-version", "unselected-costume"} {
		t.Run(change, func(t *testing.T) {
			s, p, i := characterFixture(t)
			ctx := context.Background()
			j, e := s.RequestAsset(ctx, characterRequest(p, i.Revision, "hero", target()))
			if e != nil {
				t.Fatal(e)
			}
			unrelated := LayerTarget{"page-01", "p1", "setting"}
			other, e := s.RequestAsset(ctx, characterRequest(p, i.Revision, "background", unrelated))
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
				t.Fatal(e)
			}
			body := characterFixtureJSON
			switch change {
			case "selected-costume":
				body = strings.Replace(body, "orange coat", "blue coat", 1)
			case "reference-version":
				body = strings.Replace(body, `"id":"front","version":"1"`, `"id":"front","version":"2"`, 1)
			case "unselected-costume":
				body = strings.Replace(body, "white shirt", "green shirt", 1)
			case "reference-path":
				body = strings.Replace(body, `"path":"assets/hero.png"`, `"path":"assets/setting.png"`, 1)
			}
			writeCharacter(t, p, body)
			_, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision})
			if change == "unselected-costume" {
				if e != nil {
					t.Fatalf("unrelated state invalidated draft: %v", e)
				}
				return
			}
			if e == nil {
				t.Fatal("changed character dependency accepted")
			}
			if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, other.ID, i.Revision}); e != nil {
				t.Fatalf("unrelated draft invalidated: %v", e)
			}
		})
	}
}

func TestCharacterStrictPackage(t *testing.T) {
	for _, mutation := range []struct{ name, old, new string }{
		{"numeric-version", `"version":"1"`, `"version":1`},
		{"case-alias-version", `"version":"1"`, `"version":"1","Version":"2"`},
		{"noncanonical-description", `"description":`, `"DESCRIPTION":`},
		{"nested-state-alias", `"id":"coat","version":"1"`, `"id":"coat","version":"1","Version":"2"`},
		{"nested-reference-casing", `"license":`, `"LICENSE":`},
		{"unknown-field", `"schema":`, `"vendor":"hidden","schema":`},
		{"missing-license", `"license":"CC0-1.0"`, `"license":""`},
		{"escaping-reference", `"path":"assets/hero.png"`, `"path":"../outside.png"`},
		{"unknown-costume", `"id":"coat"`, `"id":"other"`},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			s, p, i := characterFixture(t)
			writeCharacter(t, p, strings.Replace(characterFixtureJSON, mutation.old, mutation.new, 1))
			if _, e := s.RequestAsset(context.Background(), characterRequest(p, i.Revision, "strict", target())); e == nil {
				t.Fatal("invalid character package accepted")
			}
		})
	}
}
func TestCharacterProtectedPinSurvivesPackageChange(t *testing.T) {
	s, p, i := characterFixture(t)
	ctx := context.Background()
	rev := applyOp(t, s, p, i.Revision, Operation{Target: target(), Action: "review"}).Revision
	approved := applyOp(t, s, p, rev, Operation{Target: target(), Action: "approve", Artifact: "assets/hero.png"})
	writeCharacter(t, p, strings.Replace(characterFixtureJSON, "orange coat", "blue coat", 1))
	view, e := s.Inspect(ctx, InspectRequest{ProjectFile: p})
	if e != nil {
		t.Fatal(e)
	}
	if view.Layers[targetKey(target())].Pin != approved.Layers[targetKey(target())].Pin {
		t.Fatal("approved pin changed")
	}
	if _, e = s.Build(ctx, BuildRequest{ProjectFile: p, PageID: "page-01", Width: 120, Height: 180, Output: filepath.Join(filepath.Dir(p), "approved.png")}); e != nil {
		t.Fatal(e)
	}
}

func TestCharacterQueuedDraftAndStatusTrackChanges(t *testing.T) {
	s, p, i := characterFixture(t)
	ctx := context.Background()
	j, e := s.RequestAsset(ctx, characterRequest(p, i.Revision, "queued", target()))
	if e != nil {
		t.Fatal(e)
	}
	writeCharacter(t, p, strings.Replace(characterFixtureJSON, "orange coat", "blue coat", 1))
	status, e := s.Job(ctx, JobRequest{p, j.ID})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(status)
	if !strings.Contains(string(b), `"character_status":"changed"`) {
		t.Errorf("dependency changes not visible: %s", b)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	status, e = s.Job(ctx, JobRequest{p, j.ID})
	if e != nil {
		t.Fatal(e)
	}
	if status.State != "failed" {
		t.Fatalf("stale queued draft executed: %s", status.State)
	}
}

func TestCharacterReferenceBytesInvalidateWithoutVersionBump(t *testing.T) {
	s, p, i := characterFixture(t)
	ctx := context.Background()
	ref := filepath.Join(filepath.Dir(p), "assets", "reference.png")
	b, e := os.ReadFile(filepath.Join(filepath.Dir(p), "assets", "hero.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(ref, b, 0600); e != nil {
		t.Fatal(e)
	}
	writeCharacter(t, p, strings.Replace(characterFixtureJSON, `"path":"assets/hero.png"`, `"path":"assets/reference.png"`, 1))
	j, e := s.RequestAsset(ctx, characterRequest(p, i.Revision, "reference-bytes", target()))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RunJobs(ctx, RunJobsRequest{p, 1}); e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile(filepath.Join(filepath.Dir(p), "assets", "setting.png"))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(ref, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SelectCandidate(ctx, SelectCandidateRequest{p, j.ID, i.Revision}); e == nil {
		t.Fatal("changed reference bytes accepted")
	}
}

func TestCharacterRejectsAmbiguousReferenceIDs(t *testing.T) {
	s, p, i := characterFixture(t)
	var body map[string]any
	if e := json.Unmarshal([]byte(characterFixtureJSON), &body); e != nil {
		t.Fatal(e)
	}
	refs := body["references"].([]any)
	body["references"] = append(refs, refs[0])
	b, _ := json.Marshal(body)
	writeCharacter(t, p, string(b))
	if _, e := s.RequestAsset(context.Background(), characterRequest(p, i.Revision, "duplicate", target())); e == nil {
		t.Fatal("duplicate reference IDs accepted")
	}
}

func TestCreateImmutableCharacterPackage(t *testing.T) {
	s, p, _ := editFixture(t)
	ctx := context.Background()
	pkg := model.CharacterPackage{Schema: "paneltree/character/v1", ID: "patrick", Version: "v1", Description: "Silver-haired cleric", References: []model.CharacterReference{{ID: "front", Version: "v1", Path: "assets/hero.png", License: "artist-owned", Attribution: "Creator", Description: "Front view"}}}
	path, e := s.CreateCharacter(ctx, p, pkg)
	if e != nil {
		t.Fatal(e)
	}
	if path != "characters/authored/patrick/v1.json" {
		t.Fatal(path)
	}
	pkg.Description = "Changed"
	if _, e = s.CreateCharacter(ctx, p, pkg); e == nil {
		t.Fatal("published package overwritten")
	}
	data, e := os.ReadFile(filepath.Join(filepath.Dir(p), path))
	if e != nil || !strings.Contains(string(data), "Silver-haired cleric") {
		t.Fatal("original lost", e)
	}
	pkg.Version = "v2"
	pkg.References[0].Path = "../../secret.png"
	if _, e = s.CreateCharacter(ctx, p, pkg); e == nil {
		t.Fatal("external reference accepted")
	}
}
