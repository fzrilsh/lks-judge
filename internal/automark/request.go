package automark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const requestTimeout = 10 * time.Second

// DefaultConcurrency bounds how many participant servers are marked at once.
const DefaultConcurrency = 6

// response is a captured HTTP result. A transport failure sets status 0 and
// networkError, so evaluate records a failure instead of the run crashing.
type response struct {
	status       int
	json         any
	networkError string
}

// doRequest performs one HTTP call with a hard timeout. JSON bodies only.
func doRequest(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body map[string]any) response {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	var rdr io.Reader
	hasBody := body != nil && (method == "POST" || method == "PUT" || method == "PATCH")
	if hasBody {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return response{status: 0, networkError: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	if err != nil {
		msg := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			msg = "timeout"
		}
		return response{status: 0, networkError: msg}
	}
	defer func() { _ = res.Body.Close() }()

	var decoded any
	raw, _ := io.ReadAll(res.Body)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &decoded) // non-JSON leaves decoded nil, like the JS null
	}
	return response{status: res.StatusCode, json: decoded}
}

// buildHost assembles {scheme}://{ip}:{port}{path}. A target Host wins outright.
// Ports 80/443 and an empty path are omitted.
func buildHost(base Base, t Target) string {
	if t.Host != "" {
		return strings.TrimRight(t.Host, "/")
	}
	scheme := base.Scheme
	if scheme == "" {
		scheme = "http"
	}
	ip := t.IP
	if ip == "" {
		ip = "localhost"
	}
	host := scheme + "://" + ip
	if base.Port != 0 && !((scheme == "http" && base.Port == 80) || (scheme == "https" && base.Port == 443)) {
		host += fmt.Sprintf(":%d", base.Port)
	}
	if p := strings.Trim(base.Path, "/"); p != "" {
		host += "/" + p
	}
	return host
}

func noteFor(pct float64, notes []Note) string {
	for _, n := range notes {
		if pct >= n.Min {
			return n.Text
		}
	}
	return ""
}
