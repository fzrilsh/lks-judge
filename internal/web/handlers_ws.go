package web

import (
	"net/http"

	"github.com/fzrilsh/lks-judge/internal/realtime"
	"github.com/fzrilsh/lks-judge/internal/store"
)

// HandleWS upgrades /ws. Auth is optional by design (spec §8): a participant session
// cookie or an allowlisted jury IP gets every event; anyone else connects anonymously
// and receives only CountdownTick and ScoreUpdated.
func HandleWS(st *store.Store, hub *realtime.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authenticated := false

		if cookie, err := r.Cookie("participant_session"); err == nil {
			if p, err := st.ValidateSession(cookie.Value); err == nil && p != nil {
				authenticated = true
			}
		}
		if !authenticated {
			_, authenticated = juryAllowed(st, r)
		}

		realtime.ServeWS(hub, authenticated, w, r)
	}
}
