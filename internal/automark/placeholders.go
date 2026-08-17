package automark

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// uniqid returns a short unique-ish token, matching the JS uniqid() shape
// (hex time + hex random) closely enough for placeholder substitution.
func uniqid() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return strconv.FormatInt(time.Now().UnixMilli(), 16) + hex.EncodeToString(b[:])
}

// withPlaceholders deep-copies value, replacing {{uniqid}} in every string leaf
// with a fresh token so bodies like "email{{uniqid}}@x.id" stay unique per run.
func withPlaceholders(value any) any {
	switch t := value.(type) {
	case string:
		if strings.Contains(t, "{{uniqid}}") {
			return strings.ReplaceAll(t, "{{uniqid}}", uniqid())
		}
		return t
	case []any:
		out := make([]any, len(t))
		for i, v := range t {
			out[i] = withPlaceholders(v)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = withPlaceholders(v)
		}
		return out
	default:
		return value
	}
}
