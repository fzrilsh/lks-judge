package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/scoring"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// parseAllowedIPs converts the comma-separated textarea value into the JSON
// array stored in the DB (and read by reloadAllowedNets), rejecting any entry
// that is not a valid IP or CIDR so a typo cannot silently lock the jury out.
func parseAllowedIPs(raw string) (string, error) {
	var ips []string
	for part := range strings.SplitSeq(raw, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		if net.ParseIP(s) == nil {
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", fmt.Errorf("invalid IP or CIDR: %q", s)
			}
		}
		ips = append(ips, s)
	}
	if ips == nil {
		ips = []string{}
	}
	b, err := json.Marshal(ips)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

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
		allowedIPs, err := parseAllowedIPs(allowedIPs)
		if err != nil {
			log.Printf("reject allowed_ips: %v", err)
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Log an allowlist change: it directly controls jury access, so leave a trail.
		if prev := st.CompetitionCache.Load(); prev == nil || prev.AllowedIPs != allowedIPs {
			old := "[]"
			if prev != nil {
				old = prev.AllowedIPs
			}
			log.Printf("allowed_ips changed: %s -> %s ip=%s", old, allowedIPs, clientIP(r))
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

// HandleResetPOST performs a nuclear reset: snapshot the DB first (preReset, a
// backup callback injected by main so web needn't import backup), wipe all data
// and cached state, then clear the on-disk file/submission/upload dirs. The
// backups and logs dirs are left alone: backups are the only recovery path and
// the log rotator holds an open handle. A failed backup is logged, not fatal.
func HandleResetPOST(st *store.Store, cache *scoring.Cache, hub *realtime.Hub, dataDir string, preReset func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if preReset != nil {
			if err := preReset(); err != nil {
				log.Printf("reset: pre-reset backup failed, continuing: %v", err)
			}
		}
		if err := st.Reset(); err != nil {
			log.Printf("reset: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, d := range []string{"files", "submissions", "uploads_tmp"} {
			p := filepath.Join(dataDir, d)
			if err := os.RemoveAll(p); err != nil {
				log.Printf("reset: remove %s: %v", d, err)
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				log.Printf("reset: recreate %s: %v", d, err)
			}
		}
		cache.Clear()
		hub.Broadcast(realtime.EvFormOpened, map[string]any{"status": false})
		log.Printf("jury reset: wiped all data ip=%s", clientIP(r))
		http.Redirect(w, r, "/jury/?setup=1", http.StatusSeeOther)
	}
}
