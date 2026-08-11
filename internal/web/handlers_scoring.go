package web

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/scoring"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// HandleScoringGET renders the jury raw-score matrix.
func HandleScoringGET(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		participants, err := st.ListParticipants(comp.ID)
		if err != nil {
			log.Printf("list participants: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		modules, err := st.ListModules(comp.ID)
		if err != nil {
			log.Printf("list modules: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		scores, err := st.ScoresByParticipantModule(comp.ID)
		if err != nil {
			log.Printf("scores by pm: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// map[pid]map[mid]*float64 for the template cells
		cell := make(map[int64]map[int64]*float64, len(scores))
		for pid, ms := range scores {
			inner := make(map[int64]*float64, len(ms))
			for mid, v := range ms {
				vv := v
				inner[mid] = &vv
			}
			cell[pid] = inner
		}
		saved := r.URL.Query().Get("saved") == "1"
		errMsg := r.URL.Query().Get("error")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.Scoring(comp, participants, modules, cell, saved, errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render scoring: %v", err)
		}
	}
}

// HandleScoringPOST parses the whole grid, validates each value 0..100, upserts
// (blank clears to NULL), refreshes the leaderboard cache and broadcasts.
func HandleScoringPOST(st *store.Store, cache *scoring.Cache, hub *realtime.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/", http.StatusSeeOther)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		// First pass: parse and validate the whole grid before any DB write, so an
		// invalid cell cannot leave a partial commit with a stale cache/broadcast.
		var updates []store.ScoreUpdate
		for key, vals := range r.Form {
			if !strings.HasPrefix(key, "score_") || len(vals) == 0 {
				continue
			}
			var pid, mid int64
			if _, err := fmt.Sscanf(key, "score_%d_%d", &pid, &mid); err != nil {
				continue
			}
			raw := strings.TrimSpace(vals[0])
			if raw == "" {
				updates = append(updates, store.ScoreUpdate{ParticipantID: pid, ModuleID: mid, Score: nil})
				continue
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
				http.Redirect(w, r, "/jury/scoring?error="+url.QueryEscape("nilai harus 0..100"), http.StatusSeeOther)
				return
			}
			vv := v
			updates = append(updates, store.ScoreUpdate{ParticipantID: pid, ModuleID: mid, Score: &vv})
		}
		// Second pass: all values valid, commit them in one transaction (all or nothing).
		if err := st.UpsertScores(updates); err != nil {
			log.Printf("upsert scores: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := cache.Refresh(st, comp.ID); err != nil {
			log.Printf("refresh leaderboard cache: %v", err)
		}
		log.Printf("scores saved: comp=%d ip=%s", comp.ID, clientIP(r))
		hub.Broadcast(realtime.EvScoreUpdated, map[string]any{})
		http.Redirect(w, r, "/jury/scoring?saved=1", http.StatusSeeOther)
	}
}

// HandleScoringExportPDF streams the scaled-results PDF.
func HandleScoringExportPDF(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		comp := st.CompetitionCache.Load()
		if comp == nil {
			http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
			return
		}
		totals, err := st.ListParticipantTotals(comp.ID)
		if err != nil {
			log.Printf("list totals: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		entries := make([]scoring.Entry, len(totals))
		for i, t := range totals {
			entries[i] = scoring.Entry{
				ParticipantID: t.ParticipantID, Name: t.Name, School: t.School,
				PCNumber: t.PCNumber, TotalRaw: t.TotalRaw,
			}
		}
		left, right := PDFLogos()
		pdf, err := scoring.PDF(comp, scoring.Rank(entries), left, right)
		if err != nil {
			log.Printf("build pdf: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="scale-results.pdf"`)
		log.Printf("pdf export: comp=%d ip=%s", comp.ID, clientIP(r))
		_, _ = w.Write(pdf)
	}
}

// HandleLeaderboardGET renders the public leaderboard shell (HTML) or the JSON
// snapshot when the path is /leaderboard.json.
func HandleLeaderboardGET(st *store.Store, cache *scoring.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(cache.Snapshot())
			return
		}
		comp := st.CompetitionCache.Load()
		if comp == nil {
			comp = &model.Competition{Name: "LKS"}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.Leaderboard(comp).Render(r.Context(), w); err != nil {
			log.Printf("render leaderboard: %v", err)
		}
	}
}
