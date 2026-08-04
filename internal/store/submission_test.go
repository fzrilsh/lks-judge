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
	pid, err := s.CreateParticipant(compID, "Peserta", "SMK", nil, "x")
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
