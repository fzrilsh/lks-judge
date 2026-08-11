package web

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/scoring"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/dashboardview"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// dashChart is serialized to a <script type="application/json"> block the
// dashboard JS reads. Kept flat so Chart.js config stays in JS, data in Go.
type dashChart struct {
	TimingLabels []string `json:"timingLabels"` // "0-5", "5-10", ... minutes after start
	TimingCounts []int    `json:"timingCounts"`
	ModuleLabels []string `json:"moduleLabels"`
	ModuleCounts []int    `json:"moduleCounts"`
}

// HandleDashboardJuryGET renders the jury dashboard at /jury/. Because "GET
// /jury/" is a prefix pattern, it must reject any deeper unmatched path.
func HandleDashboardJuryGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jury/" {
			http.NotFound(w, r)
			return
		}
		c := st.CompetitionCache.Load()
		if c == nil {
			http.Redirect(w, r, "/jury/competition?setup=1", http.StatusSeeOther)
			return
		}

		data, err := buildDashboard(st, c, time.Now())
		if err != nil {
			log.Printf("dashboard: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := templates.DashboardJuryPage(data).Render(r.Context(), w); err != nil {
			log.Printf("render dashboard: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

// buildDashboard gathers dashboard data. Split out from the handler so the test
// can assert on the struct without an HTTP round trip.
func buildDashboard(st *store.Store, c *model.Competition, now time.Time) (*dashboardview.Data, error) {
	modules, err := st.ListModules(c.ID)
	if err != nil {
		return nil, err
	}
	participants, err := st.ListParticipants(c.ID)
	if err != nil {
		return nil, err
	}
	subs, err := st.ListSubmissions(c.ID)
	if err != nil {
		return nil, err
	}
	totals, err := st.ListParticipantTotals(c.ID)
	if err != nil {
		return nil, err
	}

	d := &dashboardview.Data{
		Comp:            c,
		Modules:         len(modules),
		Participants:    len(participants),
		Submissions:     len(subs),
		SubmissionSlots: len(participants) * len(modules),
	}

	if c.CurrentModuleID != nil {
		if m, err := st.GetModuleByID(*c.CurrentModuleID); err == nil {
			d.CurrentModule = m
		}
	}

	d.Seconds, _ = realtime.TimeLeft(c, now)
	d.FormOpen = realtime.FormOpen(c, d.Seconds)

	pName := make(map[int64]string, len(participants))
	for _, p := range participants {
		pName[p.ID] = p.Name
		if p.PCNumber != nil {
			d.Seated++
		}
	}

	// Participants with at least one submission.
	submitted := make(map[int64]bool, len(subs))
	for _, s := range subs {
		submitted[s.ParticipantID] = true
	}
	d.NotSubmitted = len(participants) - len(submitted)

	// Top 3 by WSI, read-only.
	entries := make([]scoring.Entry, len(totals))
	for i, t := range totals {
		entries[i] = scoring.Entry{
			ParticipantID: t.ParticipantID, Name: t.Name, School: t.School,
			PCNumber: t.PCNumber, TotalRaw: t.TotalRaw,
		}
	}
	ranked := scoring.Rank(entries)
	if len(ranked) > 3 {
		ranked = ranked[:3]
	}
	d.Top = ranked

	d.Activity = buildActivity(subs, participants, pName, now)
	d.ChartJSON = buildChartJSON(subs, modules, c, now)
	return d, nil
}

// buildActivity derives a recent-activity feed from submission timestamps and
// participant seat/IP registration, newest first, capped at 20. ponytail:
// derived feed, not an audit trail. Login/logout are invisible because they are
// not persisted; add an activity_events table if a real trail is ever needed.
func buildActivity(subs []*model.Submission, participants []*model.Participant, pName map[int64]string, now time.Time) []dashboardview.Activity {
	var acts []dashboardview.Activity
	for _, s := range subs {
		when := s.UpdatedAt
		if s.SubmittedAt != nil {
			when = *s.SubmittedAt
		}
		acts = append(acts, dashboardview.Activity{
			When: when,
			Icon: "upload_file",
			Text: pName[s.ParticipantID] + " mengumpulkan " + s.Name,
		})
	}
	for _, p := range participants {
		if p.IPAddress != nil && *p.IPAddress != "" {
			acts = append(acts, dashboardview.Activity{
				When: p.UpdatedAt,
				Icon: "person",
				Text: p.Name + " aktif dari " + *p.IPAddress,
			})
		}
	}
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].When.After(acts[j].When) })
	if len(acts) > 20 {
		acts = acts[:20]
	}
	return acts
}

// buildChartJSON builds the submission-timing histogram (minutes after start,
// 5-minute buckets) and the per-module submission counts, serialized for the
// dashboard JS.
func buildChartJSON(subs []*model.Submission, modules []*model.Module, c *model.Competition, now time.Time) string {
	const bucketMin = 5
	const buckets = 24 // 0..120 min; the last bucket absorbs anything later

	timing := make([]int, buckets)
	start, okStart := realtime.At(now.Format("2006-01-02"), c.StartTime)
	for _, s := range subs {
		if s.SubmittedAt == nil || !okStart {
			continue
		}
		min := int(s.SubmittedAt.Sub(start).Minutes())
		if min < 0 {
			min = 0
		}
		b := min / bucketMin
		if b >= buckets {
			b = buckets - 1
		}
		timing[b]++
	}
	timingLabels := make([]string, buckets)
	for i := range timingLabels {
		timingLabels[i] = strconv.Itoa(i*bucketMin) + "-" + strconv.Itoa((i+1)*bucketMin)
	}

	perModule := make(map[int64]int, len(modules))
	for _, s := range subs {
		perModule[s.ModuleID]++
	}
	modLabels := make([]string, len(modules))
	modCounts := make([]int, len(modules))
	for i, m := range modules {
		modLabels[i] = m.Name
		modCounts[i] = perModule[m.ID]
	}

	b, err := json.Marshal(dashChart{
		TimingLabels: timingLabels, TimingCounts: timing,
		ModuleLabels: modLabels, ModuleCounts: modCounts,
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}
