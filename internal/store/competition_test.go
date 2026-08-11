package store

import (
	"errors"
	"sync"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func newTestStore(t *testing.T) (*Store, int64) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert competition: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c == nil {
		t.Fatal("competition cache nil after upsert")
	}
	return s, c.ID
}

func currentModuleID(t *testing.T, s *Store) int64 {
	t.Helper()
	c, err := s.GetCompetition()
	if err != nil {
		t.Fatalf("get competition: %v", err)
	}
	if c.CurrentModuleID == nil {
		return -1
	}
	return *c.CurrentModuleID
}

func TestUpsertModuleByNameAutoSetsFirstCurrent(t *testing.T) {
	s, compID := newTestStore(t)

	first, err := s.UpsertModuleByName(compID, "MA")
	if err != nil {
		t.Fatalf("create MA: %v", err)
	}
	if err := s.AutoSetCurrentIfFirst(compID, first); err != nil {
		t.Fatalf("auto set: %v", err)
	}
	if got := currentModuleID(t, s); got != first {
		t.Fatalf("current module = %d, want %d", got, first)
	}

	second, err := s.UpsertModuleByName(compID, "MB")
	if err != nil {
		t.Fatalf("create MB: %v", err)
	}
	if err := s.AutoSetCurrentIfFirst(compID, second); err != nil {
		t.Fatalf("auto set 2: %v", err)
	}
	if got := currentModuleID(t, s); got != first {
		t.Fatalf("current module moved to %d, want it to stay %d", got, first)
	}

	// Same name must not insert a second row.
	again, err := s.UpsertModuleByName(compID, "MA")
	if err != nil {
		t.Fatalf("re-upsert MA: %v", err)
	}
	if again != first {
		t.Fatalf("re-upsert MA gave id %d, want %d", again, first)
	}
	mods, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mods) != 2 {
		t.Fatalf("got %d modules, want 2", len(mods))
	}
}

func TestGenerateModulesSkipsTakenNames(t *testing.T) {
	s, compID := newTestStore(t)

	if _, err := s.GenerateModules(compID, 2); err != nil {
		t.Fatalf("generate 2: %v", err)
	}
	if _, err := s.GenerateModules(compID, 2); err != nil {
		t.Fatalf("generate 2 again: %v", err)
	}

	mods, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	seen := map[string]bool{}
	orders := map[int]bool{}
	for _, m := range mods {
		if seen[m.Name] {
			t.Errorf("duplicate module name %q", m.Name)
		}
		seen[m.Name] = true
		if orders[m.Order] {
			t.Errorf("duplicate order %d", m.Order)
		}
		orders[m.Order] = true
		names = append(names, m.Name)
	}
	if len(mods) != 4 {
		t.Fatalf("got %v (%d modules), want 4", names, len(mods))
	}
	for _, want := range []string{"MA", "MB", "MC", "MD"} {
		if !seen[want] {
			t.Errorf("missing %s, got %v", want, names)
		}
	}
}

func TestGenerateModulesRejectsBadTotalAndExhaustion(t *testing.T) {
	s, compID := newTestStore(t)

	for _, total := range []int{0, -1, len(moduleSuffixes) + 1} {
		if _, err := s.GenerateModules(compID, total); err == nil {
			t.Errorf("total=%d accepted, want error", total)
		}
	}

	if _, err := s.GenerateModules(compID, len(moduleSuffixes)); err != nil {
		t.Fatalf("generate all: %v", err)
	}
	if _, err := s.GenerateModules(compID, 1); err == nil {
		t.Error("generate past MA-MG succeeded, want exhaustion error")
	}
}

func TestSetCurrentModuleRejectsForeignAndMissingIDs(t *testing.T) {
	s, compID := newTestStore(t)

	mods, err := s.GenerateModules(compID, 2)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := s.SetCurrentModule(compID, mods[1].ID); err != nil {
		t.Fatalf("set current: %v", err)
	}
	if got := currentModuleID(t, s); got != mods[1].ID {
		t.Fatalf("current = %d, want %d", got, mods[1].ID)
	}

	if err := s.SetCurrentModule(compID, 999999); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("missing id error = %v, want ErrModuleNotFound", err)
	}
	// Module of another competition must not be selectable.
	if err := s.SetCurrentModule(compID+1, mods[0].ID); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("foreign competition error = %v, want ErrModuleNotFound", err)
	}
	if got := currentModuleID(t, s); got != mods[1].ID {
		t.Fatalf("failed set changed current to %d", got)
	}
}

func TestDeleteModuleClearsCurrentAndRefreshesCache(t *testing.T) {
	s, compID := newTestStore(t)

	mods, err := s.GenerateModules(compID, 2)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Deleting a non-current module leaves current intact.
	if err := s.DeleteModule(mods[1].ID); err != nil {
		t.Fatalf("delete non-current: %v", err)
	}
	if got := currentModuleID(t, s); got != mods[0].ID {
		t.Fatalf("current = %d after deleting non-current, want %d", got, mods[0].ID)
	}

	if err := s.DeleteModule(mods[0].ID); err != nil {
		t.Fatalf("delete current: %v", err)
	}
	if got := currentModuleID(t, s); got != -1 {
		t.Fatalf("current = %d after deleting it, want NULL", got)
	}
	if c := s.CompetitionCache.Load(); c == nil || c.CurrentModuleID != nil {
		t.Fatal("cache still reports a current module after delete")
	}
	if err := s.SetCurrentModule(compID, mods[0].ID); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("set deleted module error = %v, want ErrModuleNotFound", err)
	}
}

func TestRenameModuleKeepsOrder(t *testing.T) {
	s, compID := newTestStore(t)

	mods, err := s.GenerateModules(compID, 2)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	target := mods[1]

	if err := s.RenameModule(target.ID, "Renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	list, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range list {
		if m.ID != target.ID {
			continue
		}
		if m.Name != "Renamed" {
			t.Errorf("name = %q, want Renamed", m.Name)
		}
		if m.Order != target.Order {
			t.Errorf("order = %d, want %d", m.Order, target.Order)
		}
		return
	}
	t.Fatalf("module %d missing after rename", target.ID)
}

// Order is computed inside the INSERT; concurrent creates must not collide.
func TestUpsertModuleByNameConcurrentOrdersAreUnique(t *testing.T) {
	s, compID := newTestStore(t)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.UpsertModuleByName(compID, string(rune('A'+i)))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	mods, err := s.ListModules(compID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mods) != n {
		t.Fatalf("got %d modules, want %d", len(mods), n)
	}
	orders := make(map[int]bool, n)
	for _, m := range mods {
		if orders[m.Order] {
			t.Errorf("order %d assigned twice", m.Order)
		}
		orders[m.Order] = true
	}
	if len(orders) != n {
		t.Fatalf("%d distinct orders across %d modules", len(orders), n)
	}
}

func TestUpsertCompetitionPreservesCountdownState(t *testing.T) {
	s, id := newTestStore(t)

	if _, err := s.Writer.Exec(
		`UPDATE competitions SET status='paused', remaining_seconds=847 WHERE id=?`, id,
	); err != nil {
		t.Fatalf("seed countdown state: %v", err)
	}

	if err := s.UpsertCompetition(&model.Competition{
		Name: "Renamed", Level: "Provinsi", AllowedIPs: `["10.0.0.1"]`,
		StartDate: "2026-02-01", EndDate: "2026-02-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	c, err := s.GetCompetition()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.Name != "Renamed" {
		t.Errorf("settings not saved: name=%q", c.Name)
	}
	if c.Status != "paused" {
		t.Errorf("status clobbered: got %q want paused", c.Status)
	}
	if c.RemainingSeconds == nil || *c.RemainingSeconds != 847 {
		t.Errorf("remaining_seconds clobbered: got %v want 847", c.RemainingSeconds)
	}
}

func TestResetWipesEverything(t *testing.T) {
	s, compID := newTestStore(t)
	if _, err := s.GenerateModules(compID, 2); err != nil {
		t.Fatalf("modules: %v", err)
	}
	pc := 1
	pid, err := s.CreateParticipant(compID, "Hana", "S", &pc, hashPw(t, "pw"), "pw")
	if err != nil {
		t.Fatalf("participant: %v", err)
	}
	token, err := s.CreateSession(pid)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	if err := s.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}

	for _, tbl := range []string{"competitions", "modules", "participants", "sessions", "scores"} {
		var n int
		if err := s.Reader.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if n != 0 {
			t.Errorf("%s not empty after reset: %d rows", tbl, n)
		}
	}
	if s.CompetitionCache.Load() != nil {
		t.Error("competition cache not cleared after reset")
	}
	if _, err := s.ValidateSession(token); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("session still valid after reset: %v", err)
	}
}
