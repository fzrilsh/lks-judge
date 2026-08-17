package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/fzrilsh/lks-judge/internal/automark"
	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/scoring"
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
		modules, err := st.ListModules(comp.ID)
		if err != nil {
			log.Printf("list modules: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		if err := templates.AutomarkPage(comp, string(raw), targets, modules, saved, errMsg).Render(r.Context(), w); err != nil {
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
			http.Redirect(w, r, "/jury/automark?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
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
			// Detached context: r.Context() is canceled the moment this handler
			// returns the 202, which would abort every request before it is sent.
			ctx := context.Background()
			automark.Run(ctx, &cfg, targets, automark.DefaultConcurrency, func(res automark.ParticipantResult) {
				hub.Broadcast(realtime.EvAutomarkResult, res)
			}, func(p automark.Progress) {
				hub.Broadcast(realtime.EvAutomarkProgress, p)
			})
			hub.Broadcast(realtime.EvAutomarkDone, map[string]any{"count": len(targets)})
		}()

		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"status":"started","targets":%d}`, len(targets))
	}
}

// automarkApplyReq is the body POSTed by the "apply scores" button: the module
// to write into, whether to add to or replace the existing cell, and the raw
// total per participant from the finished run.
type automarkApplyReq struct {
	ModuleID int64  `json:"module_id"`
	Mode     string `json:"mode"` // "replace" or "add"
	Scores   []struct {
		ParticipantID int64   `json:"participant_id"`
		Total         float64 `json:"total"`
	} `json:"scores"`
}

// HandleAutomarkApplyPOST writes the run's raw totals into a chosen module's
// score cells. mode=replace overwrites; mode=add sums onto the existing score.
// The module is passed explicitly (not read from current_module_id) so the
// browser can confirm which module before sending.
func HandleAutomarkApplyPOST(st *store.Store, cache *scoring.Cache, hub *realtime.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Error(w, "no competition", http.StatusBadRequest)
			return
		}
		var req automarkApplyReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Mode != "add" && req.Mode != "replace" {
			http.Error(w, "mode must be add or replace", http.StatusBadRequest)
			return
		}
		mod, err := st.GetModuleByID(req.ModuleID)
		if err != nil || mod == nil || mod.CompetitionID != comp.ID {
			http.Error(w, "module not found in this competition", http.StatusBadRequest)
			return
		}
		if len(req.Scores) == 0 {
			http.Error(w, "no scores to apply", http.StatusBadRequest)
			return
		}

		var existing map[int64]float64
		if req.Mode == "add" {
			byPM, err := st.ScoresByParticipantModule(comp.ID)
			if err != nil {
				log.Printf("scores by pm: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			existing = make(map[int64]float64, len(byPM))
			for pid, ms := range byPM {
				existing[pid] = ms[req.ModuleID]
			}
		}

		updates := make([]store.ScoreUpdate, 0, len(req.Scores))
		for _, s := range req.Scores {
			v := s.Total
			if req.Mode == "add" {
				v += existing[s.ParticipantID]
			}
			vv := v
			updates = append(updates, store.ScoreUpdate{ParticipantID: s.ParticipantID, ModuleID: req.ModuleID, Score: &vv})
		}
		if err := st.UpsertScores(updates); err != nil {
			log.Printf("automark apply upsert: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := cache.Refresh(st, comp.ID); err != nil {
			log.Printf("refresh leaderboard cache: %v", err)
		}
		log.Printf("automark scores applied: comp=%d module=%d mode=%s n=%d ip=%s", comp.ID, req.ModuleID, req.Mode, len(updates), clientIP(r))
		hub.Broadcast(realtime.EvScoreUpdated, map[string]any{})
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","applied":%d}`, len(updates))
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
		out = append(out, automark.Target{ParticipantID: p.ID, PCNumber: pc, IP: *p.IPAddress})
	}
	return out
}
