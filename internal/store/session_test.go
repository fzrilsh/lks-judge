package store

import (
	"testing"
	"time"

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

// TestDeleteExpiredSessions fences the sweep: only rows with a past expiry are
// deleted, and their tokens are evicted from the cache (which ValidateSession
// serves without an expiry check). The lifetime sentinel row survives.
func TestDeleteExpiredSessions(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 1
	id, err := s.CreateParticipant(compID, "Gita", "S", &pc, hashPw(t, "pw"), "pw")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// live sentinel session via the normal path
	live, err := s.CreateSession(id)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	// backdated row inserted directly (CreateSession always writes the 9999 sentinel)
	expired := "expired-token-" + t.Name()
	if _, err := s.Writer.Exec(
		`INSERT INTO sessions (token, owner_id, expires_at) VALUES (?, ?, ?)`,
		expired, id, time.Now().Add(-time.Hour).UTC(),
	); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	// prime the cache with the expired token so we can prove eviction
	p, err := s.GetParticipantByID(id)
	if err != nil {
		t.Fatalf("load participant: %v", err)
	}
	sessionCache.Store(expired, p)

	tokens, err := s.DeleteExpiredSessions(time.Now().UTC())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != expired {
		t.Fatalf("expected only %q swept, got %v", expired, tokens)
	}
	if _, ok := sessionCache.Load(expired); ok {
		t.Fatal("expired token still in cache after sweep")
	}
	// live session untouched in DB and cache
	if _, err := s.ValidateSession(live); err != nil {
		t.Fatalf("live session broken by sweep: %v", err)
	}
	var n int
	if err := s.Reader.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 session left, got %d", n)
	}
}
