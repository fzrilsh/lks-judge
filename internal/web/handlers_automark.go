package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/fzrilsh/lks-judge/internal/automark"
	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// automarkRunning guards against two concurrent runs (one shared client pool,
// one WS event stream). A run either starts or is told one is in flight.
var automarkRunning atomic.Bool

// HandleAutomarkGET renders the automark page with the saved config.
func HandleAutomarkGET(st *store.Store, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/competition?setup=1", http.StatusSeeOther)
			return
		}
		s, err := automark.Load(dataDir)
		if err != nil {
			log.Printf("automark load: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		raw, _ := json.MarshalIndent(s.Config, "", "  ")
		participants, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		targets := automarkTargets(participants)
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.AutomarkPage(comp, string(raw), targets, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render automark: %v", err)
		}
	}
}

// HandleAutomarkSavePOST validates and persists the pasted/builder JSON config.
func HandleAutomarkSavePOST(st *store.Store, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		cfg, err := automark.ParseConfig([]byte(r.FormValue("config")))
		if err != nil {
			http.Redirect(w, r, "/jury/automark?error=invalid+JSON:+"+err.Error(), http.StatusSeeOther)
			return
		}
		if err := automark.Save(dataDir, &automark.Store{Config: cfg}); err != nil {
			log.Printf("automark save: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/jury/automark?saved=1", http.StatusSeeOther)
	}
}

// HandleAutomarkRunPOST starts a bulk run in the background and streams
// per-participant results over WS (jury only). Returns 202 immediately; the
// browser listens for AutomarkResult / AutomarkDone events. Only one run at a
// time (409 otherwise).
func HandleAutomarkRunPOST(st *store.Store, hub *realtime.Hub, dataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Error(w, "no competition", http.StatusBadRequest)
			return
		}
		s, err := automark.Load(dataDir)
		if err != nil || len(s.Config.Groups) == 0 {
			http.Error(w, "no automark config saved", http.StatusBadRequest)
			return
		}
		participants, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		targets := automarkTargets(participants)
		if len(targets) == 0 {
			http.Error(w, "no participants have an IP address yet", http.StatusBadRequest)
			return
		}
		if !automarkRunning.CompareAndSwap(false, true) {
			http.Error(w, "a run is already in progress", http.StatusConflict)
			return
		}

		cfg := s.Config
		go func() {
			defer automarkRunning.Store(false)
			automark.Run(r.Context(), &cfg, targets, automark.DefaultConcurrency, func(res automark.ParticipantResult) {
				hub.Broadcast(realtime.EvAutomarkResult, res)
			})
			hub.Broadcast(realtime.EvAutomarkDone, map[string]any{"count": len(targets)})
		}()

		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"status":"started","targets":%d}`, len(targets))
	}
}

// automarkTargets builds one target per participant that has a recorded IP.
// Participants without an IP (never logged in) are skipped: there is nothing to
// hit. pc_number labels the result; the IP is the only per-target difference.
func automarkTargets(participants []*model.Participant) []automark.Target {
	out := make([]automark.Target, 0, len(participants))
	for _, p := range participants {
		if p.IPAddress == nil || *p.IPAddress == "" {
			continue
		}
		pc := "-"
		if p.PCNumber != nil {
			pc = fmt.Sprintf("%02d", *p.PCNumber)
		}
		out = append(out, automark.Target{PCNumber: pc, IP: *p.IPAddress})
	}
	return out
}
