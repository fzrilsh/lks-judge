package store

import "testing"

func TestListParticipantTotals(t *testing.T) {
	s, compID := newTestStore(t)

	f := func(v float64) *float64 { return &v }
	mkP := func(name string, pc int) int64 {
		id, err := s.CreateParticipant(compID, name, "SchoolX", &pc, "hash", "plain")
		if err != nil {
			t.Fatalf("create participant %s: %v", name, err)
		}
		return id
	}
	mkM := func(name string) int64 {
		id, err := s.UpsertModuleByName(compID, name)
		if err != nil {
			t.Fatalf("upsert module %s: %v", name, err)
		}
		return id
	}
	score := func(pid, mid int64, v float64) {
		if err := s.UpsertScore(pid, mid, f(v)); err != nil {
			t.Fatalf("upsert score: %v", err)
		}
	}

	pA := mkP("Alice", 1)
	pB := mkP("Bob", 2)
	mkP("Carol", 3) // no scores at all
	mA := mkM("MA")
	mB := mkM("MB")

	score(pA, mA, 50.5)
	score(pA, mB, 29.5) // Alice total 80.0
	score(pB, mA, 70.0) // Bob total 70.0
	// Carol: no rows -> total 0

	got, err := s.ListParticipantTotals(compID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	// ordered by pc_number: Alice(1), Bob(2), Carol(3)
	want := []struct {
		name  string
		total float64
	}{{"Alice", 80.0}, {"Bob", 70.0}, {"Carol", 0.0}}
	for i, w := range want {
		if got[i].Name != w.name || got[i].TotalRaw != w.total {
			t.Errorf("row %d = {%s, %v}, want {%s, %v}", i, got[i].Name, got[i].TotalRaw, w.name, w.total)
		}
	}
}
