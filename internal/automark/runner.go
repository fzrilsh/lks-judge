package automark

import (
	"context"
	"maps"
	"net/http"
	"sync"
)

// GroupResult aggregates one group's assertions for one participant.
type GroupResult struct {
	GroupID    string            `json:"group_id"`
	GroupName  string            `json:"group_name"`
	Score      float64           `json:"score"`
	Max        float64           `json:"max"`
	Note       string            `json:"note"`
	Assertions []AssertionResult `json:"assertions"`
}

// ParticipantResult is the full scored run for one target.
type ParticipantResult struct {
	PCNumber   string        `json:"pc_number"`
	Host       string        `json:"host"`
	TotalScore float64       `json:"total_score"`
	TotalMax   float64       `json:"total_max"`
	Pct        float64       `json:"pct"`
	Note       string        `json:"note"`
	Groups     []GroupResult `json:"groups"`
	Err        string        `json:"err,omitempty"`
}

// runParticipant executes every group/assertion against one target with an
// isolated auth token. A token-invalidating assertion (logout) that passes
// marks the token stale, so the next authed request lazily re-logs in, which
// is why single-login-upfront breaks against a suite that includes logout.
func runParticipant(ctx context.Context, client *http.Client, cfg *Config, t Target) ParticipantResult {
	host := buildHost(cfg.Base, t)
	res := ParticipantResult{PCNumber: t.PCNumber, Host: host}

	var token string
	tokenStale := true // force a login before the first authed request

	login := func() {
		body := map[string]any{}
		maps.Copy(body, cfg.Auth.Login.Body)
		if t.Auth != nil { // per-target credential override (testing only)
			if t.Auth.Email != "" {
				body["email"] = t.Auth.Email
			}
			if t.Auth.Password != "" {
				body["password"] = t.Auth.Password
			}
		}
		method := cfg.Auth.Login.Method
		if method == "" {
			method = "POST"
		}
		r := doRequest(ctx, client, method, host+cfg.Auth.Login.Endpoint, nil, withPlaceholders(body).(map[string]any))
		tokenStale = false
		if s, ok := getPath(r.json, cfg.Auth.TokenPath).(string); ok {
			token = s
		} else {
			token = ""
		}
	}

	for _, g := range cfg.Groups {
		gr := GroupResult{GroupID: g.GroupID, GroupName: g.GroupName}
		for _, a := range g.Assertions {
			headers := map[string]string{}
			if a.RequiresAuth {
				if tokenStale || token == "" {
					login()
				}
				headers["Authorization"] = "Bearer " + token
			}
			var body map[string]any
			if a.Request != nil && a.Request.Body != nil {
				body = withPlaceholders(a.Request.Body).(map[string]any)
			}
			resp := doRequest(ctx, client, a.Method, host+a.Endpoint, headers, body)
			ar := evaluate(a, resp)
			if a.InvalidatesToken && ar.Passed {
				tokenStale = true
			}
			gr.Assertions = append(gr.Assertions, ar)
			gr.Score += ar.Score
			gr.Max += ar.MaxScore
		}
		gr.Note = noteFor(pct(gr.Score, gr.Max), cfg.Grading.GroupNotes)
		res.Groups = append(res.Groups, gr)
		res.TotalScore += gr.Score
		res.TotalMax += gr.Max
	}
	res.Pct = pct(res.TotalScore, res.TotalMax)
	res.Note = noteFor(res.Pct, cfg.Grading.TotalNotes)
	return res
}

func pct(score, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return score / max * 100
}

// Run marks every target bounded-parallel. onResult (may be nil) fires once per
// finished participant, in completion order, for live progress. The returned
// slice is in target order regardless of completion order.
func Run(ctx context.Context, cfg *Config, targets []Target, concurrency int, onResult func(ParticipantResult)) []ParticipantResult {
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	// One client, shared: connection pooling across targets. No cookie jar, so
	// each request is stateless and auth rides only the Bearer header.
	client := &http.Client{}

	results := make([]ParticipantResult, len(targets))
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t Target) {
			defer wg.Done()
			defer func() { <-sem }()
			r := runParticipant(ctx, client, cfg, t)
			results[i] = r
			if onResult != nil {
				mu.Lock()
				onResult(r)
				mu.Unlock()
			}
		}(i, t)
	}
	wg.Wait()
	return results
}
