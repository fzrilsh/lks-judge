package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// TestModuleExportZipScopedToModule asserts the per-module ZIP contains only that
// module's submissions, laid out {pc}-{participant}/{file} (no module subfolder).
func TestModuleExportZipScopedToModule(t *testing.T) {
	s, compID := newTestStore(t)
	mods, err := s.GenerateModules(compID, 2)
	if err != nil {
		t.Fatalf("generate modules: %v", err)
	}
	p1 := seedParticipant(t, s, compID, 1, "pw12345")
	p2 := seedParticipant(t, s, compID, 2, "pw12345")

	// Real files on disk so io.Copy has bytes to stream.
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	seed := func(id string, pid, mid int64, name, path string) {
		if _, err := s.UpsertSubmission(&model.Submission{
			ID: id, ParticipantID: pid, ModuleID: mid, Name: name, FilePath: path,
		}); err != nil {
			t.Fatalf("upsert submission %s: %v", id, err)
		}
	}
	seed("s1", p1, mods[0].ID, "a.zip", write("f1", "AAA")) // module 0
	seed("s2", p2, mods[0].ID, "b.zip", write("f2", "BBBB"))
	seed("s3", p1, mods[1].ID, "c.zip", write("f3", "CC")) // module 1: must be excluded

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/jury/submissions/module/%d/export.zip", mods[0].ID), nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.SetPathValue("moduleID", fmt.Sprintf("%d", mods[0].ID))
	HandleModuleSubmissionsExportZipGET(s)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	want := []string{"01-P/a.zip", "02-P/b.zip"}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for _, w := range want {
		if !got[w] {
			t.Fatalf("missing entry %q in %v", w, got)
		}
	}
	if got["01-P/c.zip"] {
		t.Fatal("module 1 submission leaked into module 0 zip")
	}
}

// TestModuleExportZipUnknownModule returns 404 for a module ID not in the competition.
func TestModuleExportZipUnknownModule(t *testing.T) {
	s, _ := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jury/submissions/module/9999/export.zip", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.SetPathValue("moduleID", "9999")
	HandleModuleSubmissionsExportZipGET(s)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
