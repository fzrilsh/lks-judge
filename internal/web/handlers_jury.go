package web

import (
	"log"
	"net/http"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

func HandleJuryGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := st.CompetitionCache.Load()
		if c == nil {
			c = &model.Competition{Status: "waiting", AllowedIPs: "[]"}
		}
		saved := r.URL.Query().Get("saved") == "1"
		if err := templates.CompetitionPage(c, saved).Render(r.Context(), w); err != nil {
			log.Printf("render competition: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

func HandleJuryPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		startTime := r.FormValue("start_time")
		endTime := r.FormValue("end_time")
		allowedIPs := r.FormValue("allowed_ips")
		if allowedIPs == "" {
			allowedIPs = "[]"
		}

		c := &model.Competition{
			Name:       r.FormValue("name"),
			Level:      r.FormValue("level"),
			AllowedIPs: allowedIPs,
			StartDate:  r.FormValue("start_date"),
			EndDate:    r.FormValue("end_date"),
		}
		if startTime != "" {
			c.StartTime = &startTime
		}
		if endTime != "" {
			c.EndTime = &endTime
		}

		if err := st.UpsertCompetition(c); err != nil {
			log.Printf("upsert competition: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/?saved=1", http.StatusSeeOther)
	}
}
