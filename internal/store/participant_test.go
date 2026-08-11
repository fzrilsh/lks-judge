package store

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func hashPw(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 8)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

// plainPw derefs the *string plain password, returning "" for nil.
func plainPw(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestCreateParticipantRoundtripsPlainPassword(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 3
	id, err := s.CreateParticipant(compID, "Ana", "SMKN 1", &pc, hashPw(t, "pw12345"), "pw12345")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	byID, err := s.GetParticipantByID(id)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	byPC, err := s.GetParticipantByPCNumber(compID, pc)
	if err != nil {
		t.Fatalf("by pc: %v", err)
	}
	list, err := s.ListParticipants(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if plainPw(byID.PlainPassword) != "pw12345" || plainPw(byPC.PlainPassword) != "pw12345" {
		t.Fatalf("plain password lost: byID=%q byPC=%q", plainPw(byID.PlainPassword), plainPw(byPC.PlainPassword))
	}
	if len(list) != 1 || plainPw(list[0].PlainPassword) != "pw12345" {
		t.Fatalf("list plain password lost: %+v", list)
	}
}

func TestScanParticipantNullPlainPassword(t *testing.T) {
	s, compID := newTestStore(t)
	if _, err := s.Writer.Exec(
		`INSERT INTO participants(competition_id, name, school, password, created_at, updated_at)
		 VALUES (?, 'NoPlain', '', 'hash', datetime('now'), datetime('now'))`, compID,
	); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	list, err := s.ListParticipants(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || plainPw(list[0].PlainPassword) != "" {
		t.Fatalf("want empty plain password, got %+v", list)
	}
}

func TestUpsertParticipantByNameInsertThenUpdate(t *testing.T) {
	s, compID := newTestStore(t)
	pc1 := 1
	id, plain, err := s.UpsertParticipantByName(compID, "Budi", "OldSchool", &pc1, nil, hashPw(t, "first"), "first")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if plain != "first" {
		t.Fatalf("insert should return plain, got %q", plain)
	}

	pc2 := 7
	ip := "10.0.0.9"
	id2, plain2, err := s.UpsertParticipantByName(compID, "Budi", "NewSchool", &pc2, &ip, hashPw(t, "second"), "second")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if id2 != id {
		t.Fatalf("upsert created a new row: %d != %d", id2, id)
	}
	if plain2 != "" {
		t.Fatalf("update should return empty plain, got %q", plain2)
	}

	got, err := s.GetParticipantByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.School != "NewSchool" || got.PCNumber == nil || *got.PCNumber != 7 {
		t.Fatalf("update not applied: %+v", got)
	}
	if got.IPAddress == nil || *got.IPAddress != "10.0.0.9" {
		t.Fatalf("ip not updated: %v", got.IPAddress)
	}
	if plainPw(got.PlainPassword) != "first" {
		t.Fatalf("plain password should survive update, got %q", plainPw(got.PlainPassword))
	}
	list, _ := s.ListParticipants(compID)
	if len(list) != 1 {
		t.Fatalf("upsert duplicated row: %d rows", len(list))
	}
}

func TestUpdateParticipantIPEvictsSessionCache(t *testing.T) {
	s, compID := newTestStore(t)
	pc := 1
	id, err := s.CreateParticipant(compID, "Cici", "S", &pc, hashPw(t, "pw12345"), "pw12345")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := s.CreateSession(id)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	// warm the cache
	if _, err := s.ValidateSession(token); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := s.UpdateParticipantIP(id, "192.168.1.50"); err != nil {
		t.Fatalf("update ip: %v", err)
	}
	// next validate must reload from DB and see the new IP
	p, err := s.ValidateSession(token)
	if err != nil {
		t.Fatalf("validate 2: %v", err)
	}
	if p.IPAddress == nil || *p.IPAddress != "192.168.1.50" {
		t.Fatalf("cache not evicted, IP = %v", p.IPAddress)
	}
}

func TestShuffleSeatsAssignsUniqueSeatsAndPersists(t *testing.T) {
	s, compID := newTestStore(t)
	for i := range 5 {
		if _, err := s.CreateParticipant(compID, string(rune('A'+i)), "S", nil, hashPw(t, "pw"), "pw"); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	ps, err := s.ListParticipants(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	results := ShuffleSeats(ps)
	if err := s.UpdateParticipantSeats(results); err != nil {
		t.Fatalf("update seats: %v", err)
	}

	after, _ := s.ListParticipants(compID)
	seen := map[int]bool{}
	for _, p := range after {
		if p.PCNumber == nil {
			t.Fatalf("participant %d unseated", p.ID)
		}
		if *p.PCNumber < 1 || *p.PCNumber > 5 {
			t.Fatalf("seat out of range: %d", *p.PCNumber)
		}
		if seen[*p.PCNumber] {
			t.Fatalf("duplicate seat %d", *p.PCNumber)
		}
		seen[*p.PCNumber] = true
	}
	if len(seen) != 5 {
		t.Fatalf("want 5 unique seats, got %d", len(seen))
	}

	// Re-shuffle: reassigning numbers other rows already hold must not trip
	// UNIQUE(competition_id, pc_number).
	if err := s.UpdateParticipantSeats(ShuffleSeats(after)); err != nil {
		t.Fatalf("re-shuffle: %v", err)
	}
}
