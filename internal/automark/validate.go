package automark

import (
	"fmt"
	"strings"
)

// validMethods is the HTTP verb set the runner (doRequest) can drive.
var validMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// Normalize trims and canonicalises in place so a hand-pasted config with stray
// whitespace or a lowercase method does not fail at run time.
func Normalize(c *Config) {
	c.Base.Path = strings.TrimSpace(c.Base.Path)
	c.Auth.Login.Endpoint = strings.TrimSpace(c.Auth.Login.Endpoint)
	c.Auth.Login.Method = strings.ToUpper(strings.TrimSpace(c.Auth.Login.Method))
	c.Auth.TokenPath = strings.TrimSpace(c.Auth.TokenPath)
	for gi := range c.Groups {
		g := &c.Groups[gi]
		g.GroupID = strings.TrimSpace(g.GroupID)
		g.GroupName = strings.TrimSpace(g.GroupName)
		for ai := range g.Assertions {
			a := &g.Assertions[ai]
			a.Title = strings.TrimSpace(a.Title)
			a.Endpoint = strings.TrimSpace(a.Endpoint)
			a.Method = strings.ToUpper(strings.TrimSpace(a.Method))
		}
	}
}

// Validate rejects a config that would fail or silently no-op at run time.
// Errors name the offending group and 1-based assertion index.
func Validate(c Config) error {
	if len(c.Groups) == 0 {
		return fmt.Errorf("config has no groups")
	}
	requiresAuth := false
	for gi := range c.Groups {
		g := c.Groups[gi]
		if g.GroupID == "" {
			return fmt.Errorf("group %d: group_id required", gi+1)
		}
		if len(g.Assertions) == 0 {
			return fmt.Errorf("group %s: no assertions", g.GroupID)
		}
		for ai := range g.Assertions {
			a := g.Assertions[ai]
			p := fmt.Sprintf("group %s assertion %d: ", g.GroupID, ai+1)
			if a.Title == "" {
				return fmt.Errorf("%stitle required", p)
			}
			if a.Method == "" {
				return fmt.Errorf("%smethod required", p)
			}
			if !validMethods[a.Method] {
				return fmt.Errorf("%smethod %q not one of GET POST PUT PATCH DELETE", p, a.Method)
			}
			if a.Endpoint == "" {
				return fmt.Errorf("%sendpoint required", p)
			}
			if !strings.HasPrefix(a.Endpoint, "/") {
				return fmt.Errorf("%sendpoint must start with \"/\"", p)
			}
			if a.Score < 0 {
				return fmt.Errorf("%sscore must not be negative", p)
			}
			if a.Deduction != nil && *a.Deduction < 0 {
				return fmt.Errorf("%sdeduction must not be negative", p)
			}
			if a.Expected.StatusCode < 100 || a.Expected.StatusCode > 599 {
				return fmt.Errorf("%sexpected.status_code must be 100..599", p)
			}
			if a.RequiresAuth {
				requiresAuth = true
			}
		}
	}
	if requiresAuth {
		if c.Auth.Login.Endpoint == "" {
			return fmt.Errorf("auth.login.endpoint required because an assertion requires auth")
		}
		if c.Auth.TokenPath == "" {
			return fmt.Errorf("auth.tokenPath required because an assertion requires auth")
		}
	}
	return nil
}
