package store

import (
	"errors"
	"testing"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func TestUpsertSubmissionReplaces(t *testing.T) {
	s, compID := newTestStore(t)

	modID, err := s.UpsertModuleByName(compID, "MA")
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	pid, err := s.CreateParticipant(compID, "Peserta", "SMK", nil, "x", "")
	if err != nil {
		t.Fatalf("participant: %v", err)
	}

	now := time.Now().UTC()
	oldPath, err := s.UpsertSubmission(&model.Submission{
		ID: "s1", ParticipantID: pid, ModuleID: modID, Name: "a.zip",
		FilePath: "/data/submissions/1/1/s1-a.zip", SubmittedAt: &now,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if oldPath != "" {
		t.Fatalf("want empty oldPath on insert, got %q", oldPath)
	}

	// Re-submit the same module: row replaced, oldPath points at the superseded file.
	oldPath, err = s.UpsertSubmission(&model.Submission{
		ID: "s2", ParticipantID: pid, ModuleID: modID, Name: "b.zip",
		FilePath: "/data/submissions/1/1/s2-b.zip", SubmittedAt: &now,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if oldPath != "/data/submissions/1/1/s1-a.zip" {
		t.Fatalf("want old path returned, got %q", oldPath)
	}

	// UNIQUE(participant_id, module_id) must hold: exactly one row, the new one.
	list, err := s.ListSubmissions(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 submission, got %d", len(list))
	}
	if list[0].ID != "s2" || list[0].Name != "b.zip" {
		t.Fatalf("want replaced row s2/b.zip, got %+v", list[0])
	}

	got, err := s.GetSubmissionForParticipant(pid, modID)
	if err != nil {
		t.Fatalf("get for participant: %v", err)
	}
	if got.ID != "s2" {
		t.Fatalf("want s2, got %s", got.ID)
	}

	if _, err := s.GetSubmissionByID("nope"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("want ErrSubmissionNotFound, got %v", err)
	}
}

func TestScoresByParticipantModule(t *testing.T) {
	s, compID := newTestStore(t)

	f := func(v float64) *float64 { return &v }
	pA, err := s.CreateParticipant(compID, "Alice", "SMK", nil, "x", "")
	if err != nil {
		t.Fatalf("participant A: %v", err)
	}
	pB, err := s.CreateParticipant(compID, "Bob", "SMK", nil, "x", "")
	if err != nil {
		t.Fatalf("participant B: %v", err)
	}
	mA, err := s.UpsertModuleByName(compID, "MA")
	if err != nil {
		t.Fatalf("module MA: %v", err)
	}
	mB, err := s.UpsertModuleByName(compID, "MB")
	if err != nil {
		t.Fatalf("module MB: %v", err)
	}

	if err := s.UpsertScore(pA, mA, f(88.5)); err != nil {
		t.Fatalf("score A/MA: %v", err)
	}
	if err := s.UpsertScore(pB, mA, f(70.0)); err != nil {
		t.Fatalf("score B/MA: %v", err)
	}
	if err := s.UpsertScore(pB, mB, nil); err != nil { // null score must be omitted
		t.Fatalf("score B/MB nil: %v", err)
	}

	got, err := s.ScoresByParticipantModule(compID)
	if err != nil {
		t.Fatal(err)
	}
	if got[pA][mA] != 88.5 {
		t.Fatalf("A/MA = %v, want 88.5", got[pA][mA])
	}
	if got[pB][mA] != 70.0 {
		t.Fatalf("B/MA = %v, want 70.0", got[pB][mA])
	}
	// null score is not present, and A has no MB entry
	if _, ok := got[pB][mB]; ok {
		t.Fatalf("null score should be absent, got %v", got[pB][mB])
	}
	if _, ok := got[pA][mB]; ok {
		t.Fatalf("missing score should be absent, got %v", got[pA][mB])
	}
}

func TestUpsertScoresAtomic(t *testing.T) {
	s, compID := newTestStore(t)
	f := func(v float64) *float64 { return &v }

	pA, err := s.CreateParticipant(compID, "Alice", "SMK", nil, "x", "")
	if err != nil {
		t.Fatalf("participant A: %v", err)
	}
	mA, err := s.UpsertModuleByName(compID, "MA")
	if err != nil {
		t.Fatalf("module MA: %v", err)
	}

	// Batch commit: a valid cell plus a forged cell with a nonexistent module id.
	// The FK violation must roll back the whole batch, leaving no partial commit.
	err = s.UpsertScores([]ScoreUpdate{
		{ParticipantID: pA, ModuleID: mA, Score: f(80)},
		{ParticipantID: pA, ModuleID: 999999, Score: f(90)}, // bad FK
	})
	if err == nil {
		t.Fatal("want FK error on forged module id, got nil")
	}
	got, err := s.ScoresByParticipantModule(compID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[pA][mA]; ok {
		t.Fatalf("first cell must have rolled back, got %v", got[pA][mA])
	}

	// A clean batch lands every cell.
	if err := s.UpsertScores([]ScoreUpdate{{ParticipantID: pA, ModuleID: mA, Score: f(80)}}); err != nil {
		t.Fatalf("clean batch: %v", err)
	}
	got, _ = s.ScoresByParticipantModule(compID)
	if got[pA][mA] != 80 {
		t.Fatalf("A/MA = %v, want 80", got[pA][mA])
	}
}
