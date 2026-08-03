package store

import (
	"database/sql"
	"fmt"
	"time"
)

// SetCountdownTimes saves the schedule and re-arms the countdown: any frozen pause state is
// dropped and the status returns to waiting, so the ticker re-evaluates against the new times.
func (s *Store) SetCountdownTimes(startTime, endTime string) error {
	if _, err := s.Writer.Exec(`
		UPDATE competitions SET
		    start_time = ?, end_time = ?,
		    status = 'waiting', remaining_seconds = NULL, paused_at = NULL,
		    updated_at = ?`,
		startTime, endTime, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("set countdown times: %w", err)
	}
	return s.LoadCompetitionCache()
}

// TransitionStatus moves the competition from one status to another. The WHERE guard makes it a
// no-op when someone else already moved it, so the ticker and a jury click cannot fight.
func (s *Store) TransitionStatus(from, to string) error {
	if _, err := s.Writer.Exec(
		`UPDATE competitions SET status = ?, updated_at = ? WHERE status = ?`,
		to, time.Now().UTC(), from,
	); err != nil {
		return fmt.Errorf("transition status %s->%s: %w", from, to, err)
	}
	return s.LoadCompetitionCache()
}

// PauseCountdown freezes the remaining seconds. No-op unless the countdown is running.
func (s *Store) PauseCountdown(remaining int, at time.Time) error {
	if _, err := s.Writer.Exec(`
		UPDATE competitions SET
		    status = 'paused', remaining_seconds = ?, paused_at = ?, updated_at = ?
		WHERE status = 'running'`,
		remaining, at.UTC(), time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("pause countdown: %w", err)
	}
	return s.LoadCompetitionCache()
}

// ResumeCountdown rewrites end_date/end_time to now+remaining and clears the frozen state.
// The read happens on the Writer pool, which is single-connection, so no other write can
// interleave between reading remaining_seconds and applying it.
func (s *Store) ResumeCountdown(now time.Time) error {
	var remaining sql.NullInt64
	err := s.Writer.QueryRow(
		`SELECT remaining_seconds FROM competitions WHERE status = 'paused' LIMIT 1`,
	).Scan(&remaining)
	if err == sql.ErrNoRows {
		return nil // not paused; nothing to resume
	}
	if err != nil {
		return fmt.Errorf("read remaining seconds: %w", err)
	}

	end := now.Add(time.Duration(remaining.Int64) * time.Second)
	if _, err := s.Writer.Exec(`
		UPDATE competitions SET
		    status = 'running', end_date = ?, end_time = ?,
		    remaining_seconds = NULL, paused_at = NULL, updated_at = ?
		WHERE status = 'paused'`,
		end.Format("2006-01-02"), end.Format("15:04:05"), time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("resume countdown: %w", err)
	}
	return s.LoadCompetitionCache()
}

// StopCountdown ends the run. The schedule is kept so the jury can see what was configured;
// re-saving the form is what re-arms it.
func (s *Store) StopCountdown() error {
	if _, err := s.Writer.Exec(`
		UPDATE competitions SET
		    status = 'finished', remaining_seconds = NULL, paused_at = NULL, updated_at = ?`,
		time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("stop countdown: %w", err)
	}
	return s.LoadCompetitionCache()
}
