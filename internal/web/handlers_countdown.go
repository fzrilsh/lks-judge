package web

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

func HandleCountdownJuryGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.CountdownJuryPage(comp, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render countdown jury: %v", err)
		}
	}
}

// HandleCountdownJuryPOST saves the schedule. Saving always re-arms: any live or paused run is
// dropped so the new times take effect from a clean waiting state.
func HandleCountdownJuryPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		startTime := r.FormValue("start_time")
		endTime := r.FormValue("end_time")

		start, okStart := realtime.At(comp.StartDate, &startTime)
		end, okEnd := realtime.At(comp.EndDate, &endTime)
		if !okStart || !okEnd {
			http.Redirect(w, r, "/jury/countdown?error=start+and+end+time+are+required", http.StatusSeeOther)
			return
		}
		if !end.After(start) {
			http.Redirect(w, r, "/jury/countdown?error=end+must+be+after+start", http.StatusSeeOther)
			return
		}

		if err := st.SetCountdownTimes(startTime, endTime); err != nil {
			log.Printf("set countdown times: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/countdown?saved=1", http.StatusSeeOther)
	}
}

func HandleCountdownPause(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		remaining, _ := realtime.TimeLeft(comp, time.Now())
		if err := st.PauseCountdown(remaining, time.Now()); err != nil {
			log.Printf("pause countdown: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/countdown", http.StatusSeeOther)
	}
}

func HandleCountdownResume(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := st.ResumeCountdown(time.Now()); err != nil {
			log.Printf("resume countdown: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/countdown", http.StatusSeeOther)
	}
}

func HandleCountdownStop(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := st.StopCountdown(); err != nil {
			log.Printf("stop countdown: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/countdown", http.StatusSeeOther)
	}
}

// HandleCountdownPublicGET serves the TV page. No auth: it is meant for a projector.
func HandleCountdownPublicGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := templates.CountdownPublicPage(st.CompetitionCache.Load()).Render(r.Context(), w); err != nil {
			log.Printf("render countdown public: %v", err)
		}
	}
}

// HandleCountdownTimeGET is the 1s polling endpoint for both the jury and the TV page. It applies
// any due transition so the clock stays correct even if the server ticker is behind.
func HandleCountdownTimeGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		seconds, transitionTo := realtime.TimeLeft(comp, time.Now())
		if transitionTo != "" {
			if err := st.TransitionStatus(comp.Status, transitionTo); err != nil {
				log.Printf("countdown transition: %v", err)
			}
		}

		status := "waiting"
		if comp != nil {
			status = comp.Status
			if transitionTo != "" {
				status = transitionTo
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"seconds":` + strconv.Itoa(seconds) + `,"status":"` + status + `"}`))
	}
}
