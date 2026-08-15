package automark

import (
	"encoding/json"
	"strings"
	"testing"
)

// okAssertion is a minimal valid assertion the negative cases mutate.
func okAssertion() Assertion {
	return Assertion{Title: "t", Method: "GET", Endpoint: "/x", Score: 1,
		Expected: Expected{StatusCode: 200}}
}

func okConfig() Config {
	return Config{Groups: []Group{{GroupID: "A", Assertions: []Assertion{okAssertion()}}}}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
		want string // substring the error must contain; "" means valid
	}{
		{"clean", func(*Config) {}, ""},
		{"no groups", func(c *Config) { c.Groups = nil }, "config has no groups"},
		{"empty group_id", func(c *Config) { c.Groups[0].GroupID = "" }, "group 1: group_id required"},
		{"no assertions", func(c *Config) { c.Groups[0].Assertions = nil }, "group A: no assertions"},
		{"no title", func(c *Config) { c.Groups[0].Assertions[0].Title = "" }, "group A assertion 1: title required"},
		{"no method", func(c *Config) { c.Groups[0].Assertions[0].Method = "" }, "group A assertion 1: method required"},
		{"bad method", func(c *Config) { c.Groups[0].Assertions[0].Method = "FETCH" }, `method "FETCH" not one of`},
		{"no endpoint", func(c *Config) { c.Groups[0].Assertions[0].Endpoint = "" }, "endpoint required"},
		{"endpoint no slash", func(c *Config) { c.Groups[0].Assertions[0].Endpoint = "x" }, `endpoint must start with "/"`},
		{"negative score", func(c *Config) { c.Groups[0].Assertions[0].Score = -1 }, "score must not be negative"},
		{"negative deduction", func(c *Config) { c.Groups[0].Assertions[0].Deduction = f(-1) }, "deduction must not be negative"},
		{"zero deduction ok", func(c *Config) { c.Groups[0].Assertions[0].Deduction = f(0) }, ""},
		{"status low", func(c *Config) { c.Groups[0].Assertions[0].Expected.StatusCode = 0 }, "status_code must be 100..599"},
		{"status high", func(c *Config) { c.Groups[0].Assertions[0].Expected.StatusCode = 600 }, "status_code must be 100..599"},
		{"auth needs endpoint", func(c *Config) {
			c.Groups[0].Assertions[0].RequiresAuth = true
			c.Auth.TokenPath = "data.token"
		}, "auth.login.endpoint required"},
		{"auth needs tokenPath", func(c *Config) {
			c.Groups[0].Assertions[0].RequiresAuth = true
			c.Auth.Login.Endpoint = "/login"
		}, "auth.tokenPath required"},
		{"auth complete ok", func(c *Config) {
			c.Groups[0].Assertions[0].RequiresAuth = true
			c.Auth.Login.Endpoint = "/login"
			c.Auth.TokenPath = "data.token"
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := okConfig()
			tc.mut(&c)
			err := Validate(c)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	c := Config{
		Base: Base{Path: "  /api "},
		Auth: Auth{Login: LoginSpec{Method: " post ", Endpoint: " /login "}, TokenPath: " data.token "},
		Groups: []Group{{GroupID: " A ", GroupName: " G ", Assertions: []Assertion{
			{Title: " t ", Method: " get ", Endpoint: " /x "},
		}}},
	}
	Normalize(&c)
	if c.Base.Path != "/api" || c.Auth.Login.Method != "POST" || c.Auth.Login.Endpoint != "/login" || c.Auth.TokenPath != "data.token" {
		t.Fatalf("base/auth not normalized: %+v", c)
	}
	a := c.Groups[0].Assertions[0]
	if c.Groups[0].GroupID != "A" || c.Groups[0].GroupName != "G" || a.Title != "t" || a.Method != "GET" || a.Endpoint != "/x" {
		t.Fatalf("group/assertion not normalized: %+v", c.Groups[0])
	}
}

// TestParseConfigDeductionRoundTrip pins that nil (full-penalty) and 0
// (record-but-keep-score) survive a marshal/unmarshal distinctly.
func TestParseConfigDeductionRoundTrip(t *testing.T) {
	c := okConfig()
	c.Groups[0].Assertions = []Assertion{okAssertion(), okAssertion()}
	c.Groups[0].Assertions[1].Deduction = f(0)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"deduction":`) && !strings.Contains(string(raw), `"deduction":0`) {
		t.Fatalf("nil deduction should be omitted, got %s", raw)
	}
	got, err := ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Groups[0].Assertions[0].Deduction != nil {
		t.Fatalf("first deduction should stay nil, got %v", *got.Groups[0].Assertions[0].Deduction)
	}
	if got.Groups[0].Assertions[1].Deduction == nil || *got.Groups[0].Assertions[1].Deduction != 0 {
		t.Fatalf("second deduction should round-trip as 0, got %v", got.Groups[0].Assertions[1].Deduction)
	}
}
