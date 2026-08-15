// Package automark runs config-driven HTTP assertion suites against many
// participant servers, bounded-parallel. It is the Go port of the test.js
// prototype: data-driven config (from the jury UI), declarative auth with lazy
// re-login, and per-participant token isolation. Imports stdlib only, so it
// stays free of store/web cycles (same rule as internal/scoring/formula.go).
package automark

// Config is the whole assertion suite. It maps 1:1 to the JSON the jury pastes
// or the visual builder emits, so a saved automark.json round-trips unchanged.
type Config struct {
	Base    Base    `json:"base"`
	Auth    Auth    `json:"auth"`
	Grading Grading `json:"grading"`
	Groups  []Group `json:"groups"`
}

// Base builds each target host as {scheme}://{ip}:{port}{path}. Port and path
// are optional; the participant IP is the only per-target difference.
type Base struct {
	Scheme string `json:"scheme"`
	Port   int    `json:"port"`
	Path   string `json:"path"`
}

// Auth is the single global credential set: one login endpoint + body reused
// for every participant (see memory automark-auth-global). tokenPath is a
// dot-path into the login response, e.g. "data.user.token".
type Auth struct {
	Login     LoginSpec `json:"login"`
	TokenPath string    `json:"tokenPath"`
}

type LoginSpec struct {
	Method   string         `json:"method"`
	Endpoint string         `json:"endpoint"`
	Body     map[string]any `json:"body"`
}

// Grading maps a percentage to a human note. Entries are checked top-down;
// the first whose Min the pct clears wins, so order them high to low.
type Grading struct {
	GroupNotes []Note `json:"groupNotes"`
	TotalNotes []Note `json:"totalNotes"`
}

type Note struct {
	Min  float64 `json:"min"`
	Text string  `json:"text"`
}

type Group struct {
	GroupID    string      `json:"group_id"`
	GroupName  string      `json:"group_name"`
	Assertions []Assertion `json:"assertions"`
}

// Assertion is one request + its expected response. Deduction is the penalty
// per failed check; nil means "deduct the full score" (matches the JS default).
type Assertion struct {
	Title            string   `json:"title"`
	Method           string   `json:"method"`
	Endpoint         string   `json:"endpoint"`
	RequiresAuth     bool     `json:"requires_auth,omitempty"`
	InvalidatesToken bool     `json:"invalidates_token,omitempty"`
	Request          *Request `json:"request,omitempty"`
	Expected         Expected `json:"expected"`
	Score            float64  `json:"score"`
	Deduction        *float64 `json:"deduction,omitempty"`
}

type Request struct {
	Body map[string]any `json:"body"`
}

type Expected struct {
	StatusCode int            `json:"status_code"`
	Body       map[string]any `json:"body"`
}

// Target is one participant server. Auth (or a bare host) overrides the global
// config, used only when several targets share one server during testing; in
// production each participant differs by IP alone.
type Target struct {
	PCNumber string       `json:"pc_number"`
	IP       string       `json:"ip"`
	Host     string       `json:"host,omitempty"`
	Auth     *Credentials `json:"auth,omitempty"`
}

type Credentials struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
}
