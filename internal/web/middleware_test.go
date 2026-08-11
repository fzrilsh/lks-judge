package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fzrilsh/lks-judge/internal/model"
	"github.com/fzrilsh/lks-judge/internal/store"
	"github.com/fzrilsh/lks-judge/internal/upload"
)

// newTestStore opens a fresh store with one competition (loopback allowlist).
// Package-wide helper for all web tests.
func newTestStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert competition: %v", err)
	}
	c := s.CompetitionCache.Load()
	if c == nil {
		t.Fatal("competition cache nil after upsert")
	}
	return s, c.ID
}

func TestCSRFProtectAllowsSafeMethodsAndMatchingOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CSRFProtect(next)

	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: want 200, got %d", m, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Origin", "http://"+req.Host)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("matching origin POST: want 200, got %d", rec.Code)
	}
}

func TestCSRFProtectRejects(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := CSRFProtect(next)

	cases := []struct {
		name   string
		origin string
		refer  string
	}{
		{"no headers", "", ""},
		{"foreign origin", "http://evil.example", ""},
		{"foreign origin wins over matching referer", "http://evil.example", ""},
		{"unparseable origin", "http://[::1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			if c.name == "foreign origin wins over matching referer" {
				req.Header.Set("Referer", "http://"+req.Host+"/page")
			}
			if c.refer != "" {
				req.Header.Set("Referer", c.refer)
			}
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("want 403, got %d", rec.Code)
			}
			if got := rec.Body.String(); got != "403 Forbidden: cross-origin request rejected\n" {
				t.Fatalf("body = %q", got)
			}
		})
	}
}

func TestJuryAllowedMatching(t *testing.T) {
	cases := []struct {
		name       string
		allowedIPs string
		remoteAddr string
		want       bool
	}{
		{"single /32 match", `["10.0.0.5"]`, "10.0.0.5:1234", true},
		{"cidr /24 match", `["10.0.0.0/24"]`, "10.0.0.99:1234", true},
		{"ipv6 loopback", `["::1"]`, "[::1]:1234", true},
		{"bracketed ipv6 remoteaddr", `["fe80::1"]`, "[fe80::1]:9", true},
		{"remoteaddr no port", `["10.0.0.5"]`, "10.0.0.5", true},
		{"ip rejected", `["10.0.0.5"]`, "10.0.0.6:1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			if err := s.UpsertCompetition(&model.Competition{
				Name: "Test", Level: "Nasional", AllowedIPs: c.allowedIPs,
				StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
			}); err != nil {
				t.Fatalf("upsert: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = c.remoteAddr
			if _, ok := juryAllowed(s, req); ok != c.want {
				t.Fatalf("juryAllowed = %v, want %v", ok, c.want)
			}
		})
	}
}

func TestJuryAllowedEmptyAllowlistIsLoopbackOnly(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	if _, ok := juryAllowed(s, req); !ok {
		t.Fatal("loopback should be allowed on empty allowlist")
	}
	req.RemoteAddr = "10.0.0.5:1234"
	if _, ok := juryAllowed(s, req); ok {
		t.Fatal("non-loopback should be denied on empty allowlist")
	}
}

func TestJuryAllowedExtraNets(t *testing.T) {
	// No competition, so the DB allowlist is empty. The --jury-ip flag alone
	// must grant access, and loopback stays allowed regardless.
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if bad := s.SetExtraNets([]string{"10.0.0.0/24", "192.168.1.7"}); len(bad) != 0 {
		t.Fatalf("SetExtraNets bad = %v", bad)
	}

	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"10.0.0.42:1", true},   // in CIDR
		{"192.168.1.7:1", true}, // exact IP
		{"192.168.1.8:1", false},
		{"127.0.0.1:1", true}, // loopback is always allowed (operator box)
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.addr
		if _, ok := juryAllowed(s, req); ok != tc.want {
			t.Fatalf("%s: juryAllowed = %v, want %v", tc.addr, ok, tc.want)
		}
	}

	if bad := s.SetExtraNets([]string{"not-an-ip"}); len(bad) != 1 || bad[0] != "not-an-ip" {
		t.Fatalf("SetExtraNets should reject junk, got %v", bad)
	}
}

func TestJuryAllowedMalformedAllowlist(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Writer.Exec(`UPDATE competitions SET allowed_ips = '{oops'`); err != nil {
		t.Fatalf("set malformed: %v", err)
	}
	if err := s.LoadCompetitionCache(); err != nil {
		t.Fatalf("reload cache: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	if _, ok := juryAllowed(s, req); !ok {
		t.Fatal("malformed allowlist should fall back to loopback")
	}
	req.RemoteAddr = "10.0.0.5:1234"
	if _, ok := juryAllowed(s, req); ok {
		t.Fatal("malformed allowlist should deny non-loopback")
	}
}

func TestRequireJuryReloadsAllowlistOnUpsert(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["10.0.0.0/8"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert wide: %v", err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := RequireJury(s)(next)

	mk := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/jury", nil)
		req.RemoteAddr = "10.1.2.3:1234"
		return req
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mk())
	if rec.Code != http.StatusOK {
		t.Fatalf("wide allowlist: want 200, got %d", rec.Code)
	}

	if err := s.UpsertCompetition(&model.Competition{
		Name: "Test", Level: "Nasional", AllowedIPs: `["127.0.0.1"]`,
		StartDate: "2026-01-01", EndDate: "2026-01-02", Status: "waiting",
	}); err != nil {
		t.Fatalf("upsert narrow: %v", err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, mk())
	if rec.Code != http.StatusForbidden {
		t.Fatalf("after re-upsert: want 403, got %d", rec.Code)
	}
}

func TestRequireParticipantRedirects(t *testing.T) {
	s, compID := newTestStore(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetParticipant(r) == nil {
			t.Error("participant not in context")
		}
		w.WriteHeader(http.StatusOK)
	})
	h := RequireParticipant(s)(next)

	// no cookie
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("no cookie: want 303, got %d", rec.Code)
	}

	// garbage cookie
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: "garbage"})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("garbage cookie: want 303, got %d", rec.Code)
	}

	// valid session
	pid := seedParticipant(t, s, compID, 1, "pw12345")
	token, err := s.CreateSession(pid)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: token})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid session: want 200, got %d", rec.Code)
	}
}

func TestRequireUploaderPrecedence(t *testing.T) {
	s, compID := newTestStore(t)

	// valid participant session -> participant identity
	pid := seedParticipant(t, s, compID, 1, "pw12345")
	token, err := s.CreateSession(pid)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var gotRole string
	var gotID int64
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := upload.UploaderFrom(r.Context()); ok {
			gotRole, gotID = u.Role, u.ID
		}
		w.WriteHeader(http.StatusOK)
	})
	h := RequireUploader(s)(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/upload/init", nil)
	req.RemoteAddr = "10.0.0.5:1234" // not loopback: forces session path
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: token})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("participant session: want 200, got %d", rec.Code)
	}
	if gotRole != "participant" || gotID != pid {
		t.Fatalf("participant session: want participant/%d, got %s/%d", pid, gotRole, gotID)
	}

	// loopback jury IP + a VALID participant cookie -> participant wins (session
	// checked first, so a participant on an allowlisted IP is not misread as jury)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload/init", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: token})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback with valid cookie: want 200, got %d", rec.Code)
	}
	if gotRole != "participant" || gotID != pid {
		t.Fatalf("loopback with valid cookie: want participant/%d, got %s/%d", pid, gotRole, gotID)
	}

	// loopback jury IP + a stale (invalid) cookie -> jury identity (session fails,
	// falls through to IP check)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload/init", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.AddCookie(&http.Cookie{Name: "participant_session", Value: "stale-token"})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback with stale cookie: want 200, got %d", rec.Code)
	}
	if gotRole != "jury" || gotID != 0 {
		t.Fatalf("loopback with stale cookie: want jury/0, got %s/%d", gotRole, gotID)
	}

	// no cookie + loopback -> jury identity
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload/init", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback jury: want 200, got %d", rec.Code)
	}

	// neither -> 401 JSON
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/upload/init", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth: want 401, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	if got := rec.Body.String(); got != `{"error":"unauthenticated"}` {
		t.Fatalf("body = %q", got)
	}
}
