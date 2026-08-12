// Package dashboardview holds the plain data types the jury dashboard handler
// builds and the templ template renders. It lives in its own package because
// templ templates cannot import the web package (web imports templates), so the
// shared struct must sit below both. It imports model and scoring only.
package dashboardview

import (
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/scoring"
)

// Activity is one derived timeline row. There is no persisted event log
// (login/logout are not recorded), so activity is reconstructed from data that
// does exist: submission timestamps and participant seat/IP changes.
type Activity struct {
	When time.Time
	Icon string
	Text string
}

// Data is everything the jury dashboard renders, assembled from existing store
// list methods (no new SQL: for ~16 participants a len() and an in-Go loop is
// the right tool).
type Data struct {
	Comp            *model.Competition
	CurrentModule   *model.Module
	Modules         int
	Participants    int
	Seated          int
	NotSubmitted    int
	Submissions     int
	SubmissionSlots int
	Seconds         int
	FormOpen        bool
	Top             []scoring.Entry
	Activity        []Activity
	ChartJSON       string
}
