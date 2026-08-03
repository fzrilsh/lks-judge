package store

import (
	"testing"
	"time"
)

func status(t *testing.T, s *Store) string {
	t.Helper()
	c := s.CompetitionCache.Load()
	if c == nil {
		t.Fatal("competition cache nil")
	}
	return c.Status
}

func TestSetCountdownTimesReArms(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.SetCountdownTimes("08:00", "12:00"); err != nil {
		t.Fatalf("set times: %v", err)
	}
	if err := s.PauseCountdown(0, time.Now()); err != nil { // no-op, status is waiting
		t.Fatalf("pause: %v", err)
	}
	if err := s.TransitionStatus("waiting", "running"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := s.PauseCountdown(500, time.Now()); err != nil {
		t.Fatalf("pause: %v", err)
	}

	if err := s.SetCountdownTimes("09:00", "13:00"); err != nil {
		t.Fatalf("re-save times: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c.Status != "waiting" {
		t.Fatalf("want waiting, got %s", c.Status)
	}
	if c.RemainingSeconds != nil || c.PausedAt != nil {
		t.Fatalf("frozen state not cleared: remaining=%v pausedAt=%v", c.RemainingSeconds, c.PausedAt)
	}
	if c.StartTime == nil || *c.StartTime != "09:00" || c.EndTime == nil || *c.EndTime != "13:00" {
		t.Fatalf("times not saved: %v %v", c.StartTime, c.EndTime)
	}
}

func TestTransitionStatusGuarded(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.TransitionStatus("running", "finished"); err != nil {
		t.Fatalf("guarded transition: %v", err)
	}
	if got := status(t, s); got != "waiting" {
		t.Fatalf("guard miss should be a no-op, got %s", got)
	}

	if err := s.TransitionStatus("waiting", "running"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if got := status(t, s); got != "running" {
		t.Fatalf("want running, got %s", got)
	}
}

func TestPauseResume(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.SetCountdownTimes("08:00", "23:00"); err != nil {
		t.Fatalf("set times: %v", err)
	}
	if err := s.TransitionStatus("waiting", "running"); err != nil {
		t.Fatalf("transition: %v", err)
	}

	pausedAt := time.Now()
	if err := s.PauseCountdown(90, pausedAt); err != nil {
		t.Fatalf("pause: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c.Status != "paused" {
		t.Fatalf("want paused, got %s", c.Status)
	}
	if c.RemainingSeconds == nil || *c.RemainingSeconds != 90 {
		t.Fatalf("want remaining=90, got %v", c.RemainingSeconds)
	}
	if c.PausedAt == nil {
		t.Fatal("paused_at not set")
	}

	now := time.Now()
	if err := s.ResumeCountdown(now); err != nil {
		t.Fatalf("resume: %v", err)
	}
	c = s.CompetitionCache.Load()
	if c.Status != "running" {
		t.Fatalf("want running, got %s", c.Status)
	}
	if c.RemainingSeconds != nil || c.PausedAt != nil {
		t.Fatalf("frozen state not cleared: %v %v", c.RemainingSeconds, c.PausedAt)
	}
	wantEnd := now.Add(90 * time.Second)
	if c.EndDate != wantEnd.Format("2006-01-02") {
		t.Fatalf("want end_date %s, got %s", wantEnd.Format("2006-01-02"), c.EndDate)
	}
	if c.EndTime == nil || *c.EndTime != wantEnd.Format("15:04:05") {
		t.Fatalf("want end_time %s, got %v", wantEnd.Format("15:04:05"), c.EndTime)
	}
}

func TestPauseGuard(t *testing.T) {
	s, _ := newTestStore(t) // status is waiting

	if err := s.PauseCountdown(42, time.Now()); err != nil {
		t.Fatalf("pause: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c.Status != "waiting" || c.RemainingSeconds != nil {
		t.Fatalf("pause should be a no-op while waiting: %s %v", c.Status, c.RemainingSeconds)
	}
}

func TestResumeGuard(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.TransitionStatus("waiting", "running"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	before := s.CompetitionCache.Load()

	if err := s.ResumeCountdown(time.Now()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	after := s.CompetitionCache.Load()
	if after.Status != "running" || after.EndDate != before.EndDate {
		t.Fatalf("resume should be a no-op when not paused: %s %s", after.Status, after.EndDate)
	}
}

func TestStop(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.SetCountdownTimes("08:00", "12:00"); err != nil {
		t.Fatalf("set times: %v", err)
	}
	if err := s.TransitionStatus("waiting", "running"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := s.PauseCountdown(120, time.Now()); err != nil {
		t.Fatalf("pause: %v", err)
	}

	if err := s.StopCountdown(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c.Status != "finished" {
		t.Fatalf("want finished, got %s", c.Status)
	}
	if c.RemainingSeconds != nil || c.PausedAt != nil {
		t.Fatalf("frozen state not cleared: %v %v", c.RemainingSeconds, c.PausedAt)
	}
	if c.StartTime == nil || *c.StartTime != "08:00" || c.EndTime == nil || *c.EndTime != "12:00" {
		t.Fatalf("stop must keep the schedule, got %v %v", c.StartTime, c.EndTime)
	}
}
