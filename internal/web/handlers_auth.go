package web

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/realtime"
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

		// Record the client IP seen at login (deviation from spec, which sources
		// IP from the Excel import column only).
		ip, _, splitErr := net.SplitHostPort(r.RemoteAddr)
		if splitErr != nil {
			ip = r.RemoteAddr
		}
		if err := st.RecordParticipantIP(participant.ID, ip); err != nil {
			log.Printf("login: record ip: %v", err)
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

// HandleDashboard serves the participant dashboard: the active module, the
// public file list, the participant's existing submission for that module, and
// whether the submission form is currently open (competition running, inside
// the last FormOpenSeconds). The page then goes live over the WebSocket.
func HandleDashboard(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		participant := GetParticipant(r)
		if participant == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		comp := st.CompetitionCache.Load()

		var activeModule *model.Module
		var existing *model.Submission
		var publicFiles []*model.File
		formOpen := false

		if comp != nil {
			if comp.CurrentModuleID != nil {
				if m, err := st.GetModuleByID(*comp.CurrentModuleID); err == nil {
					activeModule = m
					if s, serr := st.GetSubmissionForParticipant(participant.ID, m.ID); serr == nil {
						existing = s
					} else if !errors.Is(serr, store.ErrSubmissionNotFound) {
						log.Printf("dashboard: get submission: %v", serr)
					}
				}
			}
			files, err := st.ListFiles(comp.ID)
			if err != nil {
				log.Printf("dashboard: list files: %v", err)
			}
			for _, f := range files {
				if f.IsPublic {
					publicFiles = append(publicFiles, f)
				}
			}
			if comp.Status == "running" {
				seconds, _ := realtime.TimeLeft(comp, time.Now())
				formOpen = seconds > 0 && seconds <= realtime.FormOpenSeconds
			}
		}

		if err := templates.Dashboard(participant, activeModule, publicFiles, existing, formOpen).Render(r.Context(), w); err != nil {
			log.Printf("render dashboard template: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}
