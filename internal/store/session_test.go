package store

import (
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func TestSessionCacheHitAndDBFallback(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	compID := s.CompetitionCache.Load().ID
	pc := 1
	id, err := s.CreateParticipant(compID, "Dedi", "S", &pc, hashPw(t, "pw"), "pw")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := s.CreateSession(id)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	// two validates: first may hit DB, both must succeed (cache path exercised)
	if _, err := s.ValidateSession(token); err != nil {
		t.Fatalf("validate 1: %v", err)
	}
	if _, err := s.ValidateSession(token); err != nil {
		t.Fatalf("validate 2: %v", err)
	}
	_ = s.Close()

	// fresh store on the same dir: cache is empty, token resolves from DB
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	p, err := s2.ValidateSession(token)
	if err != nil {
		t.Fatalf("db fallback validate: %v", err)
	}
	if p.ID != id {
		t.Fatalf("wrong participant from DB: %d != %d", p.ID, id)
	}
}

func TestDeleteSessionEvicts(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 1
	id, err := s.CreateParticipant(compID, "Eka", "S", &pc, hashPw(t, "pw"), "pw")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := s.CreateSession(id)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := s.ValidateSession(token); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := s.DeleteSession(token); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.ValidateSession(token); err == nil {
		t.Fatal("deleted session still validates")
	}
}

// TestSessionParticipantHasPlainPassword fences Fix 2: a participant loaded
// through the session cache must carry plain_password, which the deleted
// private loader dropped.
func TestSessionParticipantHasPlainPassword(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 1
	id, err := s.CreateParticipant(compID, "Fitri", "S", &pc, hashPw(t, "pw"), "secret99")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := s.CreateSession(id)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	p, err := s.ValidateSession(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if plainPw(p.PlainPassword) != "secret99" {
		t.Fatalf("session participant missing plain password, got %q", plainPw(p.PlainPassword))
	}
}
