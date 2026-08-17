package automark

import (
	"encoding/json"
	"testing"
)

// TestExampleConfigJSON keeps the Load-example payload honest: it must parse,
// normalize, and validate clean, and it must exercise the deduction distinction
// (one assertion omits it, one sets it to 0) so both branches stay covered.
func TestExampleConfigJSON(t *testing.T) {
	cfg, err := ParseConfig([]byte(ExampleConfigJSON))
	if err != nil {
		t.Fatalf("example must parse + validate clean, got %v", err)
	}
	if len(cfg.Groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(cfg.Groups))
	}

	var withZero, withNil, requiresAuth, invalidates bool
	for _, g := range cfg.Groups {
		for _, a := range g.Assertions {
			if a.Deduction == nil {
				withNil = true
			} else if *a.Deduction == 0 {
				withZero = true
			}
			if a.RequiresAuth {
				requiresAuth = true
			}
			if a.InvalidatesToken {
				invalidates = true
			}
		}
	}
	if !withNil || !withZero {
		t.Fatalf("example should show deduction both absent and 0 (nil=%v zero=%v)", withNil, withZero)
	}
	if !requiresAuth || !invalidates {
		t.Fatalf("example should exercise requires_auth and invalidates_token (auth=%v inv=%v)", requiresAuth, invalidates)
	}

	// Re-marshalling the parsed config and re-parsing must stay valid: the
	// example is byte-representable by the same round-trip the builder uses.
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseConfig(raw); err != nil {
		t.Fatalf("example does not survive a marshal round trip: %v", err)
	}
}
