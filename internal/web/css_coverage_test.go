package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCSSCoverage asserts every MD3 design-token class used in the compiled
// templates and JS resolves to a rule in the generated app.css. Guards against
// a template referencing an undefined token (blank styling at runtime).
func TestCSSCoverage(t *testing.T) {
	css, err := os.ReadFile("static/css/app.css")
	if err != nil {
		t.Fatalf("read app.css: %v", err)
	}
	cssStr := string(css)

	// The design-token class stems this project relies on.
	stems := []string{
		"surface-container", "on-surface", "on-surface-variant",
		"headline-large", "headline-medium", "title-large", "title-medium",
		"label-large", "label-medium", "label-small",
		"body-large", "body-medium", "body-small",
		"font-manrope", "signature-gradient", "ambient-shadow",
		"material-symbols-outlined", "on-secondary-container",
		"error-container", "on-error", "outline-variant",
		"card", "btn-primary", "chip", "data-table", "table-wrap", "field",
		"warning-container",
	}
	for _, s := range stems {
		if !strings.Contains(cssStr, s) {
			t.Errorf("token %q used by templates but absent from app.css (regenerate: go generate ./...)", s)
		}
	}

	// Sanity: at least one _templ.go actually references font-manrope, so the
	// list above cannot silently drift to all-dead stems.
	var found bool
	_ = filepath.Walk("templates", func(p string, _ os.FileInfo, _ error) error {
		if strings.HasSuffix(p, "_templ.go") {
			b, _ := os.ReadFile(p)
			if regexp.MustCompile(`font-manrope`).Match(b) {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Error("expected font-manrope in a compiled template; scan list may be stale")
	}
}
