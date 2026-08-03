// Package realtime holds countdown timing logic and (from Phase 8) the WebSocket hub.
// It imports model only: callers pass snapshots and apply the transitions this package reports.
package realtime

import (
	"context"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

// FormOpenSeconds is the remaining-time threshold at which the submission form opens (spec §7).
const FormOpenSeconds = 1200

var timeLayouts = [2]string{"15:04:05", "15:04"}

// At combines a DATE ("2006-01-02") with a TIME ("15:04" or "15:04:05") in the local zone.
// ok is false when either part is missing or unparseable.
func At(date string, t *string) (time.Time, bool) {
	if date == "" || t == nil || *t == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if ts, err := time.ParseInLocation("2006-01-02 "+layout, date+" "+*t, time.Local); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// TimeLeft reports the seconds remaining plus the status transition the caller must apply
// ("" means none). It never mutates c.
func TimeLeft(c *model.Competition, now time.Time) (seconds int, transitionTo string) {
	if c == nil {
		return 0, ""
	}

	switch c.Status {
	case "paused":
		if c.RemainingSeconds != nil {
			return *c.RemainingSeconds, ""
		}
		return 0, ""

	case "waiting", "running":
		start, okStart := At(c.StartDate, c.StartTime)
		end, okEnd := At(c.EndDate, c.EndTime)
		if !okStart || !okEnd {
			return 0, ""
		}
		if !now.Before(end) {
			return 0, "finished"
		}
		if now.Before(start) {
			return 0, ""
		}
		left := int(end.Sub(now).Seconds())
		if c.Status == "waiting" {
			return left, "running"
		}
		return left, ""
	}

	return 0, "" // finished, or an unknown status
}

// Countdown drives the 1s server tick. Every side effect is a callback so this package
// stays store-free and Phase 8 can swap the log lines for hub broadcasts.
type Countdown struct {
	Snapshot   func() *model.Competition // current competition, nil when unset
	Transition func(to string)           // apply a status change
	FormOpened func(open bool)           // crossing the 1200s threshold
	Tick       func(seconds int)         // optional per-second observer
}

// Run ticks once per second until ctx is done.
func (cd *Countdown) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()

	last := -1
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			last = cd.step(now, last)
		}
	}
}

// step applies one tick and returns the seconds value to remember. last is -1 on the first tick.
func (cd *Countdown) step(now time.Time, last int) int {
	seconds, transitionTo := TimeLeft(cd.Snapshot(), now)

	if transitionTo != "" && cd.Transition != nil {
		cd.Transition(transitionTo)
	}
	if cd.Tick != nil {
		cd.Tick(seconds)
	}

	if cd.FormOpened != nil {
		open := seconds > 0 && seconds <= FormOpenSeconds
		wasOpen := last > 0 && last <= FormOpenSeconds
		if last == -1 {
			if open {
				cd.FormOpened(true)
			}
		} else if open != wasOpen {
			cd.FormOpened(open)
		}
	}

	return seconds
}
