package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
)

// validateAllowedIPs parses the JSON allowlist and rejects any entry that is
// not a valid IP or CIDR, so a typo cannot silently lock the jury out (a
// malformed list falls back to loopback at request time).
func validateAllowedIPs(raw string) error {
	var ips []string
	if err := json.Unmarshal([]byte(raw), &ips); err != nil {
		return fmt.Errorf("allowed_ips must be a JSON array: %w", err)
	}
	for _, s := range ips {
		if net.ParseIP(s) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(s); err == nil {
			continue
		}
		return fmt.Errorf("invalid IP or CIDR: %q", s)
	}
	return nil
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
		if allowedIPs == "" {
			allowedIPs = "[]"
		}
		if err := validateAllowedIPs(allowedIPs); err != nil {
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
