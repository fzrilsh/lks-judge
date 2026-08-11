package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// TestBuildDashboardAggregates seeds a small competition and asserts the
// derived counts, top-3, activity feed, and chart JSON.
func TestBuildDashboardAggregates(t *testing.T) {
	s, compID := newTestStore(t)

	mods, err := s.GenerateModules(compID, 2)
	if err != nil {
		t.Fatalf("generate modules: %v", err)
	}

	p1 := seedParticipant(t, s, compID, 1, "pw12345")
	p2 := seedParticipant(t, s, compID, 2, "pw12345")
	seedParticipant(t, s, compID, 3, "pw12345") // no submission, no score

	// p1 submits module 1; drives Submissions count + activity + timing chart.
	now := time.Now()
	when := now.Add(-3 * time.Minute)
	if _, err := s.UpsertSubmission(&model.Submission{
		ID: "sub-1", ParticipantID: p1, ModuleID: mods[0].ID,
		Name: "kerja-p1.zip", FilePath: "/x", SubmittedAt: &when,
	}); err != nil {
		t.Fatalf("upsert submission: %v", err)
	}

	sc1, sc2 := 90.0, 40.0
	if err := s.UpsertScore(p1, mods[0].ID, &sc1); err != nil {
		t.Fatalf("score p1: %v", err)
	}
	if err := s.UpsertScore(p2, mods[0].ID, &sc2); err != nil {
		t.Fatalf("score p2: %v", err)
	}

	// A start_time a bit before now so the submission lands in an early bucket.
	c := s.CompetitionCache.Load()
	st := now.Add(-10 * time.Minute).Format("15:04")
	c.StartTime = &st
	c.Status = "running"

	d, err := buildDashboard(s, c, now)
	if err != nil {
		t.Fatalf("buildDashboard: %v", err)
	}
	if d.Participants != 3 || d.Modules != 2 {
		t.Fatalf("participants=%d modules=%d, want 3/2", d.Participants, d.Modules)
	}
	if d.Submissions != 1 || d.SubmissionSlots != 6 {
		t.Fatalf("submissions=%d slots=%d, want 1/6", d.Submissions, d.SubmissionSlots)
	}
	if d.NotSubmitted != 2 {
		t.Fatalf("notSubmitted=%d, want 2", d.NotSubmitted)
	}
	if len(d.Top) != 3 || d.Top[0].ParticipantID != p1 {
		t.Fatalf("top wrong: %+v", d.Top)
	}
	if len(d.Activity) == 0 || !strings.Contains(d.Activity[0].Text, "kerja-p1.zip") {
		t.Fatalf("activity missing submission: %+v", d.Activity)
	}
	if !strings.Contains(d.ChartJSON, "timingCounts") || !strings.Contains(d.ChartJSON, "moduleCounts") {
		t.Fatalf("chart json malformed: %s", d.ChartJSON)
	}
}

func TestDashboardRedirectsWithoutCompetition(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jury/", nil)
	HandleDashboardJuryGET(s)(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/jury/competition?setup=1" {
		t.Fatalf("location = %q", loc)
	}
}

func TestDashboardNotFoundOnDeepPath(t *testing.T) {
	s, _ := newTestStore(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jury/nonexistent", nil)
	HandleDashboardJuryGET(s)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
