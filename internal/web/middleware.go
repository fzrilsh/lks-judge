package web

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/upload"
)

type contextKey string

const participantCtxKey contextKey = "participant"

// RequireParticipant validates session cookie and injects participant into context.
// Redirects to /login if invalid.
func RequireParticipant(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("participant_session")
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			participant, err := st.ValidateSession(cookie.Value)
			if err != nil {
				log.Printf("validate session: %v", err)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), participantCtxKey, participant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// juryAllowed reports whether the request comes from an allowlisted jury IP.
// The list is read from the competition state cache; it falls back to loopback
// when no competition is configured yet.
func juryAllowed(st *store.Store, r *http.Request) (string, bool) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr // no port
	}

	allowlist := []string{"127.0.0.1", "::1"} // fallback

	if c := st.CompetitionCache.Load(); c != nil && c.AllowedIPs != "" && c.AllowedIPs != "[]" {
		var ips []string
		if jsonErr := json.Unmarshal([]byte(c.AllowedIPs), &ips); jsonErr == nil && len(ips) > 0 {
			allowlist = ips
		}
	}

	for _, a := range allowlist {
		if ip == a {
			return ip, true
		}
	}
	return ip, false
}

// RequireJury reads AllowedIPs from competition state cache.
// Falls back to ["127.0.0.1","::1"] if no competition configured yet.
// ponytail: no IP format validation; add when jury input errors are wired
func RequireJury(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, ok := juryAllowed(st, r)
			if !ok {
				log.Printf("jury access denied for IP: %s", ip)
				http.Error(w, "403 Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GetParticipant extracts participant from context (set by RequireParticipant).
func GetParticipant(r *http.Request) *model.Participant {
	p, _ := r.Context().Value(participantCtxKey).(*model.Participant)
	return p
}

// RequireUploader authorizes /upload/* requests: a valid participant session, or
// failing that an allowlisted jury IP. On success it injects the uploader identity
// (via the upload package, so the dependency stays web -> upload). On failure it
// returns 401 JSON, since these endpoints are called from fetch(), not the browser.
func RequireUploader(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie("participant_session"); err == nil {
				if p, err := st.ValidateSession(cookie.Value); err == nil {
					ctx := upload.WithUploader(r.Context(), upload.Uploader{ID: p.ID, Role: "participant"})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if _, ok := juryAllowed(st, r); ok {
				ctx := upload.WithUploader(r.Context(), upload.Uploader{ID: 0, Role: "jury"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthenticated"}`))
		})
	}
}
