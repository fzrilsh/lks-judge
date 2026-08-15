package automark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// tokenSet is a tiny concurrent set so the mock stays dependency-free.
type tokenSet struct {
	mu sync.Mutex
	m  map[string]bool
}

func newTokenSet() *tokenSet { return &tokenSet{m: map[string]bool{}} }
func (t *tokenSet) add(s string) {
	t.mu.Lock()
	t.m[s] = true
	t.mu.Unlock()
}
func (t *tokenSet) has(s string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[s]
}
func (t *tokenSet) del(s string) {
	t.mu.Lock()
	delete(t.m, s)
	t.mu.Unlock()
}

func bearer(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(p) && h[:len(p)] == p {
		return h[len(p):]
	}
	return ""
}

// mockAPI mirrors the prototype's mock server: stateful login/logout (exercises
// lazy re-login) plus an authed GET that 401s without a valid token.
func mockAPI() *httptest.Server {
	tokens := newTokenSet()
	mux := http.NewServeMux()
	send := func(w http.ResponseWriter, code int, obj any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(obj)
	}
	decode := func(r *http.Request) map[string]any {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		return m
	}
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if decode(r)["password"] == "wrongpass" {
			send(w, 401, map[string]any{"status": "error", "message": "Email or password incorrect"})
			return
		}
		tok := "tok-" + uniqid()
		tokens.add(tok)
		send(w, 200, map[string]any{"status": "success", "message": "Logged in successfully", "data": map[string]any{"user": map[string]any{"token": tok}}})
	})
	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		if tok := bearer(r); tok == "" || !tokens.has(tok) {
			send(w, 401, map[string]any{"status": "error", "message": "Authentication required"})
			return
		}
		tokens.del(bearer(r))
		send(w, 200, map[string]any{"status": "success", "message": "Logout successfully"})
	})
	mux.HandleFunc("/api/secure", func(w http.ResponseWriter, r *http.Request) {
		if tok := bearer(r); tok == "" || !tokens.has(tok) {
			send(w, 401, map[string]any{"status": "error", "message": "Authentication required"})
			return
		}
		send(w, 200, map[string]any{"status": "success", "message": "ok", "data": map[string]any{"items": []any{}}})
	})
	return httptest.NewServer(mux)
}

func testConfig() *Config {
	return &Config{
		Auth: Auth{
			Login:     LoginSpec{Method: "POST", Endpoint: "/api/login", Body: map[string]any{"email": "u@x.id", "password": "ok"}},
			TokenPath: "data.user.token",
		},
		Grading: Grading{
			GroupNotes: []Note{{Min: 100, Text: "full"}, {Min: 0, Text: "meh"}},
			TotalNotes: []Note{{Min: 100, Text: "full"}, {Min: 0, Text: "meh"}},
		},
		Groups: []Group{{
			GroupID:   "A",
			GroupName: "Auth",
			Assertions: []Assertion{
				{Title: "login ok", Method: "POST", Endpoint: "/api/login", Score: 1,
					Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success", "data": map[string]any{"user": []any{"token"}}}}},
				{Title: "secure ok", Method: "GET", Endpoint: "/api/secure", RequiresAuth: true, Score: 1,
					Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success", "data": map[string]any{"items": map[string]any{"*": map[string]any{}}}}}},
				{Title: "logout ok", Method: "POST", Endpoint: "/api/logout", RequiresAuth: true, InvalidatesToken: true, Score: 1,
					Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success"}}},
				{Title: "secure after logout re-logs in", Method: "GET", Endpoint: "/api/secure", RequiresAuth: true, Score: 1,
					Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success"}}},
			},
		}},
	}
}

func TestRunAllPass(t *testing.T) {
	srv := mockAPI()
	defer srv.Close()

	cfg := testConfig()
	targets := []Target{{PCNumber: "01", Host: srv.URL}, {PCNumber: "02", Host: srv.URL}}

	var count int
	results := Run(context.Background(), cfg, targets, 2, func(ParticipantResult) { count++ })

	if count != 2 {
		t.Fatalf("onResult fired %d times, want 2", count)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for i, r := range results {
		if r.TotalScore != 4 || r.TotalMax != 4 {
			t.Errorf("target %d: score %v/%v, want 4/4 (deductions: %+v)", i, r.TotalScore, r.TotalMax, firstFail(r))
		}
		if r.Pct != 100 || r.Note != "full" {
			t.Errorf("target %d: pct=%v note=%q, want 100 / full", i, r.Pct, r.Note)
		}
	}
}

func TestLazyReloginAfterLogout(t *testing.T) {
	srv := mockAPI()
	defer srv.Close()

	// The 4th assertion (secure GET) runs AFTER logout invalidated the token.
	// If re-login didn't fire it would 401 and fail; assert it passed.
	res := Run(context.Background(), testConfig(), []Target{{Host: srv.URL}}, 1, nil)
	last := res[0].Groups[0].Assertions[3]
	if !last.Passed {
		t.Fatalf("post-logout secure GET failed, lazy re-login broken: %+v", last.Deductions)
	}
}

func firstFail(r ParticipantResult) []Deduction {
	for _, g := range r.Groups {
		for _, a := range g.Assertions {
			if !a.Passed {
				return a.Deductions
			}
		}
	}
	return nil
}
