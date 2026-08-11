package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A handler that sets text/html then writes an HTML body, mirroring the fixed
// HandleScoringGET / HandleLeaderboardGET. Wrapped in Gzip, it must still report
// text/html downstream (Gzip must not clobber an explicit Content-Type), proving
// the root fix (setting the header in the handler) survives compression.
func TestGzipKeepsExplicitHTMLContentType(t *testing.T) {
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><body>ok</body></html>")
	}))

	req := httptest.NewRequest(http.MethodGet, "/jury/scoring", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()

	if got := res.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html prefix (Gzip clobbered it)", ct)
	}

	gr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	body, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if !strings.Contains(string(body), "<!doctype html>") {
		t.Fatalf("decompressed body missing HTML: %q", body)
	}
}

// Without Accept-Encoding gzip, Gzip is a passthrough: no Content-Encoding
// header and the body is uncompressed.
func TestGzipPassthroughWhenNotAccepted(t *testing.T) {
	const want = "<!doctype html><html><body>ok</body></html>"
	h := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, want)
	}))

	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()

	if got := res.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (passthrough)", got)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, []byte(want)) {
		t.Fatalf("body = %q, want uncompressed %q", body, want)
	}
}
