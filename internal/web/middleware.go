package web

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/upload"
)

type contextKey string

const participantCtxKey contextKey = "participant"

// CSRFProtect rejects state-changing requests (anything but GET/HEAD/OPTIONS)
// whose Origin or Referer host does not match the request host. Jury auth is
// IP-only with no token, so without this a page on the jury's machine could
// silently POST to /jury/*. A request with neither header is rejected.
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !originMatchesHost(r) {
			http.Error(w, "403 Forbidden: cross-origin request rejected", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originMatchesHost checks the Origin header first, then Referer, against r.Host.
func originMatchesHost(r *http.Request) bool {
	for _, h := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if h == "" {
			continue
		}
		u, err := url.Parse(h)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	}
	return false // no Origin and no Referer on a state-changing request
}

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
				// A stale/absent session is normal; only log real failures.
				if !errors.Is(err, store.ErrSessionNotFound) {
					log.Printf("validate session: %v", err)
				}
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			ctx := context.WithValue(r.Context(), participantCtxKey, participant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// juryAllowed reports whether the request comes from an allowlisted jury IP.
// The parsed allowlist is read from the store (refreshed on competition write);
// an empty list falls back to loopback. Single-IP and CIDR entries are both
// stored as net.IPNet, so IPv4-mapped IPv6 normalizes via Contains.
func juryAllowed(st *store.Store, r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr // no port
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return host, false
	}

	nets := st.AllowedNets()
	extra := st.ExtraNets()
	if len(nets) == 0 && len(extra) == 0 {
		return host, remote.IsLoopback() // no competition / empty list: loopback only
	}
	for i := range nets {
		if nets[i].Contains(remote) {
			return host, true
		}
	}
	for i := range extra {
		if extra[i].Contains(remote) {
			return host, true
		}
	}
	return host, false
}

// clientIP returns the request's remote IP without the port. Falls back to the
// raw RemoteAddr when it carries no port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RequireJury reads AllowedIPs from competition state cache.
// Falls back to ["127.0.0.1","::1"] if no competition configured yet.
func RequireJury(st *store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, ok := juryAllowed(st, r)
			if !ok {
				log.Printf("jury access denied for IP: %s", ip)
				http.Error(w, "403 Forbidden", http.StatusForbidden)
				return
			}
			log.Printf("jury access: ip=%s %s %s", ip, r.Method, r.URL.Path)
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
// failing that an allowlisted jury IP. On success it injects the uploader
// identity (via the upload package, so the dependency stays web -> upload). On
// failure it returns 401 JSON, since these endpoints are called from fetch(),
// not the browser. The participant session is checked first: a jury station has
// no session (stateless IP auth), but a participant often browses from an
// allowlisted IP (loopback on a dev box, or any listed net), and jury-first
// there would misclassify them as jury (ID 0), making the submission INSERT fail
// the participant_id foreign key.
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
