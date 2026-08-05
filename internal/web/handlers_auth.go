package web

import (
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/web/templates"
	"golang.org/x/crypto/bcrypt"
)

// loginLimiter throttles password guesses per pc_number. bcrypt is deliberately
// slow, so this also caps a bcrypt-flood DoS. In-memory is fine: a restart
// clearing the counters is not a meaningful attack window on a LAN.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[int]*attempt
}

type attempt struct {
	count int
	until time.Time
}

const (
	loginMaxAttempts = 5
	loginLockWindow  = 1 * time.Minute
)

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[int]*attempt{}} }

// locked reports whether this pc_number is currently in a lockout window.
func (l *loginLimiter) locked(pc int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[pc]
	return a != nil && a.count >= loginMaxAttempts && time.Now().Before(a.until)
}

// fail records a failed attempt and (re)arms the lockout window.
func (l *loginLimiter) fail(pc int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[pc]
	if a == nil || time.Now().After(a.until) {
		a = &attempt{}
		l.attempts[pc] = a
	}
	a.count++
	a.until = time.Now().Add(loginLockWindow)
}

// success clears the counter after a valid login.
func (l *loginLimiter) success(pc int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, pc)
}

// HandleLoginGET serves the login form.
func HandleLoginGET(w http.ResponseWriter, r *http.Request) {
	errorMsg := ""
	switch r.URL.Query().Get("error") {
	case "invalid":
		errorMsg = "Nomor PC atau password salah"
	case "locked":
		errorMsg = "Terlalu banyak percobaan. Coba lagi dalam 1 menit."
	}

	if err := templates.Login(errorMsg).Render(r.Context(), w); err != nil {
		log.Printf("render login template: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// HandleLoginPOST validates credentials and creates session.
func HandleLoginPOST(st *store.Store) http.HandlerFunc {
	limiter := newLoginLimiter()
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

		if limiter.locked(pcNumber) {
			log.Printf("login: pc_number=%d locked out", pcNumber)
			http.Redirect(w, r, "/login?error=locked", http.StatusSeeOther)
			return
		}

		participant, err := st.GetParticipantByPCNumber(pcNumber)
		if err != nil {
			log.Printf("login: get participant: %v", err)
			limiter.fail(pcNumber)
			http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(participant.Password), []byte(password)); err != nil {
			log.Printf("login: bcrypt compare failed for pc_number=%d", pcNumber)
			limiter.fail(pcNumber)
			http.Redirect(w, r, "/login?error=invalid", http.StatusSeeOther)
			return
		}
		limiter.success(pcNumber)

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
			SameSite: http.SameSiteStrictMode,
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
