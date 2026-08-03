package realtime

import (
	"testing"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
)

func ptrStr(s string) *string { return &s }
func ptrInt(i int) *int       { return &i }

func TestAt(t *testing.T) {
	cases := []struct {
		name string
		date string
		time *string
		want string // "" means ok=false
	}{
		{"hh:mm", "2026-08-03", ptrStr("09:30"), "2026-08-03 09:30:00"},
		{"hh:mm:ss", "2026-08-03", ptrStr("09:30:45"), "2026-08-03 09:30:45"},
		{"nil time", "2026-08-03", nil, ""},
		{"empty time", "2026-08-03", ptrStr(""), ""},
		{"empty date", "", ptrStr("09:30"), ""},
		{"garbage", "2026-08-03", ptrStr("nope"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := At(tc.date, tc.time)
			if tc.want == "" {
				if ok {
					t.Fatalf("want ok=false, got %v", got)
				}
				return
			}
			if !ok {
				t.Fatal("want ok=true")
			}
			if got.Format("2006-01-02 15:04:05") != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got.Format("2006-01-02 15:04:05"))
			}
		})
	}
}

func comp(status, startTime, endTime string) *model.Competition {
	c := &model.Competition{
		Status:    status,
		StartDate: "2026-08-03",
		EndDate:   "2026-08-03",
	}
	if startTime != "" {
		c.StartTime = ptrStr(startTime)
	}
	if endTime != "" {
		c.EndTime = ptrStr(endTime)
	}
	return c
}

func at(hhmmss string) time.Time {
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-08-03 "+hhmmss, time.Local)
	if err != nil {
		panic(err)
	}
	return ts
}

func TestTimeLeft(t *testing.T) {
	cases := []struct {
		name           string
		c              *model.Competition
		now            time.Time
		wantSeconds    int
		wantTransition string
	}{
		{"nil competition", nil, at("10:00:00"), 0, ""},
		{"waiting before start", comp("waiting", "10:00", "11:00"), at("09:00:00"), 0, ""},
		{"waiting in window", comp("waiting", "10:00", "11:00"), at("10:30:00"), 1800, "running"},
		{"waiting past end", comp("waiting", "10:00", "11:00"), at("11:30:00"), 0, "finished"},
		{"waiting no times", comp("waiting", "", ""), at("10:30:00"), 0, ""},
		{"running before end", comp("running", "10:00", "11:00"), at("10:59:00"), 60, ""},
		{"running at end", comp("running", "10:00", "11:00"), at("11:00:00"), 0, "finished"},
		{"running past end", comp("running", "10:00", "11:00"), at("11:05:00"), 0, "finished"},
		{"finished", comp("finished", "10:00", "11:00"), at("10:30:00"), 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSeconds, gotTransition := TimeLeft(tc.c, tc.now)
			if gotSeconds != tc.wantSeconds || gotTransition != tc.wantTransition {
				t.Fatalf("want (%d, %q), got (%d, %q)",
					tc.wantSeconds, tc.wantTransition, gotSeconds, gotTransition)
			}
		})
	}
}

func TestTimeLeftPaused(t *testing.T) {
	c := comp("paused", "10:00", "11:00")
	c.RemainingSeconds = ptrInt(432)
	if s, tr := TimeLeft(c, at("23:00:00")); s != 432 || tr != "" {
		t.Fatalf("want (432, \"\"), got (%d, %q)", s, tr)
	}

	c.RemainingSeconds = nil
	if s, tr := TimeLeft(c, at("23:00:00")); s != 0 || tr != "" {
		t.Fatalf("want (0, \"\"), got (%d, %q)", s, tr)
	}
}

func TestCountdownFormOpenedCrossing(t *testing.T) {
	c := comp("running", "10:00", "11:00")
	var opens []bool

	cd := &Countdown{
		Snapshot:   func() *model.Competition { return c },
		FormOpened: func(open bool) { opens = append(opens, open) },
	}

	// end is 11:00:00, so "now" values map directly to remaining seconds.
	last := -1
	for _, now := range []string{
		"10:39:58", // 1202 left, still closed
		"10:39:59", // 1201, closed
		"10:40:00", // 1200, opens
		"10:40:01", // 1199, stays open (no double fire)
	} {
		last = cd.step(at(now), last)
	}
	if len(opens) != 1 || !opens[0] {
		t.Fatalf("want one open=true, got %v", opens)
	}

	// A resume pushing the end time back out closes the form again.
	c.EndTime = ptrStr("12:00")
	last = cd.step(at("10:40:02"), last)
	if len(opens) != 2 || opens[1] {
		t.Fatalf("want a trailing open=false, got %v", opens)
	}

	// Re-entering the window fires true once more.
	c.EndTime = ptrStr("11:00")
	_ = cd.step(at("10:41:00"), last)
	if len(opens) != 3 || !opens[2] {
		t.Fatalf("want a trailing open=true, got %v", opens)
	}
}

func TestCountdownFormOpenedFirstTickInsideWindow(t *testing.T) {
	c := comp("running", "10:00", "11:00")
	var opens []bool
	cd := &Countdown{
		Snapshot:   func() *model.Competition { return c },
		FormOpened: func(open bool) { opens = append(opens, open) },
	}

	last := cd.step(at("10:50:00"), -1) // 600 left on the very first tick
	if len(opens) != 1 || !opens[0] {
		t.Fatalf("want one open=true on first tick, got %v", opens)
	}
	_ = cd.step(at("10:50:01"), last)
	if len(opens) != 1 {
		t.Fatalf("want no further fires, got %v", opens)
	}
}

func TestCountdownTransitionAndTick(t *testing.T) {
	c := comp("waiting", "10:00", "11:00")
	var transitions []string
	var ticks []int

	cd := &Countdown{
		Snapshot:   func() *model.Competition { return c },
		Transition: func(to string) { transitions = append(transitions, to) },
		Tick:       func(seconds int) { ticks = append(ticks, seconds) },
	}

	_ = cd.step(at("10:30:00"), -1)
	if len(transitions) != 1 || transitions[0] != "running" {
		t.Fatalf("want [running], got %v", transitions)
	}
	if len(ticks) != 1 || ticks[0] != 1800 {
		t.Fatalf("want [1800], got %v", ticks)
	}
}

func TestCountdownNilCompetitionIsSafe(t *testing.T) {
	cd := &Countdown{Snapshot: func() *model.Competition { return nil }}
	if got := cd.step(at("10:00:00"), -1); got != 0 {
		t.Fatalf("want 0, got %d", got)
	}
}
