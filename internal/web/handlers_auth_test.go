package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// seedParticipant inserts a participant with a bcrypt(cost=8) hash of plainPwd
// and returns its ID.
func seedParticipant(t *testing.T, s *store.Store, compID int64, pc int, plainPwd string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPwd), 8)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	id, err := s.CreateParticipant(compID, "P", "SchoolX", &pc, string(hash), plainPwd)
	if err != nil {
		t.Fatalf("create participant: %v", err)
	}
	return id
}

func loginForm(pc, pwd string) *http.Request {
	form := url.Values{"pc_number": {pc}, "password": {pwd}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestLoginSuccessSetsCookieAndSession(t *testing.T) {
	s, compID := newTestStore(t)
	seedParticipant(t, s, compID, 1, "pw12345")

	rec := httptest.NewRecorder()
	HandleLoginPOST(s)(rec, loginForm("1", "pw12345"))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("want 303 /, got %d %q", rec.Code, rec.Header().Get("Location"))
	}

	var c *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "participant_session" {
			c = ck
		}
	}
	if c == nil {
		t.Fatal("no participant_session cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 31536000 {
		t.Fatalf("cookie attrs: HttpOnly=%v SameSite=%v MaxAge=%d", c.HttpOnly, c.SameSite, c.MaxAge)
	}
	if c.Secure {
		t.Fatal("cookie must NOT be Secure (plain HTTP on LAN)")
	}
	if _, err := s.ValidateSession(c.Value); err != nil {
		t.Fatalf("session token invalid: %v", err)
	}
}

func TestLoginRecordsIPAndCachesFreshParticipant(t *testing.T) {
	s, compID := newTestStore(t)
	pid := seedParticipant(t, s, compID, 1, "pw12345")

	rec := httptest.NewRecorder()
	req := loginForm("1", "pw12345")
	req.RemoteAddr = "10.9.8.7:5555"
	HandleLoginPOST(s)(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", rec.Code)
	}

	var token string
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "participant_session" {
			token = ck.Value
		}
	}
	// participant from session cache must carry the freshly recorded IP:
	// UpdateParticipantIP (which invalidates cache) must run before CreateSession.
	p, err := s.ValidateSession(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if p.IPAddress == nil || *p.IPAddress != "10.9.8.7" {
		t.Fatalf("recorded IP = %v, want 10.9.8.7", p.IPAddress)
	}

	// RemoteAddr without a port falls back to the raw value.
	_ = pid
	rec = httptest.NewRecorder()
	req = loginForm("1", "pw12345")
	req.RemoteAddr = "172.16.0.1"
	HandleLoginPOST(s)(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "participant_session" {
			token = ck.Value
		}
	}
	p, err = s.ValidateSession(token)
	if err != nil {
		t.Fatalf("validate 2: %v", err)
	}
	if p.IPAddress == nil || *p.IPAddress != "172.16.0.1" {
		t.Fatalf("no-port IP = %v, want 172.16.0.1", p.IPAddress)
	}
}

func TestLoginRejectsBadPasswordAndBadPCNumber(t *testing.T) {
	s, compID := newTestStore(t)
	seedParticipant(t, s, compID, 1, "pw12345")

	cases := []struct{ name, pc, pwd string }{
		{"wrong password", "1", "nope"},
		{"unknown pc", "99", "pw12345"},
		{"non-numeric pc", "abc", "pw12345"},
		{"zero pc", "0", "pw12345"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			HandleLoginPOST(s)(rec, loginForm(c.pc, c.pwd))
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?error=invalid" {
				t.Fatalf("want 303 /login?error=invalid, got %d %q", rec.Code, rec.Header().Get("Location"))
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("no cookie should be set on failed login")
			}
		})
	}
}

func TestLoginLockoutAfterFiveFailures(t *testing.T) {
	s, compID := newTestStore(t)
	seedParticipant(t, s, compID, 1, "pw12345")

	handler := HandleLoginPOST(s) // one closure keeps the limiter alive
	for n := range loginMaxAttempts {
		rec := httptest.NewRecorder()
		handler(rec, loginForm("1", "wrong"))
		if loc := rec.Header().Get("Location"); loc != "/login?error=invalid" {
			t.Fatalf("attempt %d: want invalid, got %q", n+1, loc)
		}
	}
	rec := httptest.NewRecorder()
	handler(rec, loginForm("1", "wrong"))
	if loc := rec.Header().Get("Location"); loc != "/login?error=locked" {
		t.Fatalf("6th attempt: want locked, got %q", loc)
	}
}

func TestLogoutDeletesSessionAndClearsCookie(t *testing.T) {
	s, compID := newTestStore(t)
	pid := seedParticipant(t, s, compID, 1, "pw12345")
	token, err := s.CreateSession(pid)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: token})
	HandleLogoutPOST(s)(rec, req)

	var c *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "participant_session" {
			c = ck
		}
	}
	if c == nil || c.MaxAge != -1 {
		t.Fatalf("cookie not cleared: %+v", c)
	}
	if _, err := s.ValidateSession(token); err == nil {
		t.Fatal("session should be deleted")
	}
}
