package web

import (
	"log"
	"net/http"
	"strconv"

	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
	"golang.org/x/crypto/bcrypt"
)

// HandleLoginGET serves the login form.
func HandleLoginGET(w http.ResponseWriter, r *http.Request) {
	errorMsg := ""
	if r.URL.Query().Get("error") == "invalid" {
		errorMsg = "Nomor PC atau password salah"
	}

	if err := templates.Login(errorMsg).Render(r.Context(), w); err != nil {
		log.Printf("render login template: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// HandleLoginPOST validates credentials and creates session.
func HandleLoginPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		pcNumberStr := r.FormValue("pc_number")
		password := r.FormValue("password")

		pcNumber, err := strconv.Atoi(pcNumberStr)
		if err != nil || pcNumber < 1 {
			http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
			return
		}

		participant, err := st.GetParticipantByPCNumber(pcNumber)
		if err != nil {
			log.Printf("login: get participant: %v", err)
			http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(participant.Password), []byte(password)); err != nil {
			log.Printf("login: bcrypt compare failed for pc_number=%d", pcNumber)
			http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
			return
		}

		token, err := st.CreateSession(participant.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			log.Printf("login: create session: %v", err)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "participant_session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   31536000, // 1 year (lifetime session)
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// HandleLogoutPOST deletes session and clears cookie.
func HandleLogoutPOST(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("participant_session")
		if err == nil && cookie.Value != "" {
			if err := st.DeleteSession(cookie.Value); err != nil {
				log.Printf("logout: delete session: %v", err)
			}
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "participant_session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1, // delete cookie
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// HandleDashboard serves participant dashboard.
func HandleDashboard(w http.ResponseWriter, r *http.Request) {
	participant := GetParticipant(r)
	if participant == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := templates.Dashboard(participant).Render(r.Context(), w); err != nil {
		log.Printf("render dashboard template: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
