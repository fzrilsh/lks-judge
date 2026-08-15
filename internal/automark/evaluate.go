package automark

import (
	"regexp"
	"strconv"
	"strings"
)

// Deduction is one failed check within an assertion.
type Deduction struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// AssertionResult is the scored outcome of one assertion.
type AssertionResult struct {
	Title      string      `json:"title"`
	Passed     bool        `json:"passed"`
	Score      float64     `json:"score"`
	MaxScore   float64     `json:"max_score"`
	Deductions []Deduction `json:"deductions"`
}

var numericKey = regexp.MustCompile(`^\d+$`)

// getPath resolves a dot-path like "data.user.token" against a decoded JSON
// object. Returns nil if any segment is missing.
func getPath(obj any, path string) any {
	cur := obj
	for k := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

// evaluate scores a captured response against the expected spec. status/message
// compare as scalars (message case-insensitive, mirroring the JS); every other
// body key goes through compareStructure. Each failed check deducts penalty,
// floored at 0. Semantics are preserved verbatim from test.js.
func evaluate(a Assertion, resp response) AssertionResult {
	penalty := a.Score
	if a.Deduction != nil {
		penalty = *a.Deduction
	}
	json, _ := resp.json.(map[string]any)
	if json == nil {
		json = map[string]any{}
	}
	earned := a.Score
	var deds []Deduction
	penalize := func(d Deduction) { deds = append(deds, d); earned -= penalty }

	if resp.status != a.Expected.StatusCode {
		msg := "[STATUS_CODE] Expected " + strconv.Itoa(a.Expected.StatusCode) + ", got " + strconv.Itoa(resp.status)
		if resp.networkError != "" {
			msg += " (" + resp.networkError + ")"
		}
		penalize(Deduction{Field: "status_code", Message: msg})
	}

	for key, expectedVal := range a.Expected.Body {
		switch key {
		case "status":
			actual := json["status"]
			if !scalarEqual(actual, expectedVal, false) {
				penalize(Deduction{Field: "status", Message: "[STATUS] Expected \"" + toStr(expectedVal) + "\", got \"" + toStr(actual) + "\""})
			}
		case "message":
			actual := json["message"]
			if !scalarEqual(actual, expectedVal, true) {
				penalize(Deduction{Field: "message", Message: "[MESSAGE] Expected \"" + toStr(expectedVal) + "\", got \"" + toStr(actual) + "\""})
			}
		default:
			var actual any = map[string]any{}
			if v, ok := json[key]; ok {
				actual = v
			}
			for _, e := range compareStructure(expectedVal, actual, key) {
				penalize(Deduction{Field: e.Field, Message: "[JSON_STRUCTURE] " + e.Message + " " + e.Field})
			}
		}
	}

	if earned < 0 {
		earned = 0
	}
	return AssertionResult{Title: a.Title, Passed: len(deds) == 0, Score: earned, MaxScore: a.Score, Deductions: deds}
}

func toStr(v any) string {
	if v == nil {
		return "undefined"
	}
	return printAny(v)
}

func printAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

func scalarEqual(actual, expected any, caseInsensitive bool) bool {
	as, es := printAny(actual), printAny(expected)
	if caseInsensitive {
		return strings.EqualFold(as, es)
	}
	return as == es
}

// compareStructure checks that actual has the shape expected describes.
// Rules (unchanged from the JS): numeric/array entries assert a field name
// exists; {"*": shape} asserts an array whose items each match shape; a nested
// object recurses. Only presence/shape is checked, never leaf values.
func compareStructure(expected, actual any, path string) []Deduction {
	am, ok := actual.(map[string]any)
	aArr, isArr := actual.([]any)
	if !ok && !isArr {
		return []Deduction{{Field: path, Message: "Expected object."}}
	}

	var errs []Deduction
	for key, value := range entries(expected) {
		if numericKey.MatchString(key) {
			// value is a field name that must exist in actual.
			name := printAny(value)
			if _, present := lookup(am, aArr, name); !present {
				errs = append(errs, Deduction{Field: path + "." + name, Message: "Missing field."})
			}
			continue
		}

		vm, _ := value.(map[string]any)
		if star, hasStar := vm["*"]; hasStar {
			child, present := lookup(am, aArr, key)
			arr, childIsArr := child.([]any)
			if !present || !childIsArr {
				errs = append(errs, Deduction{Field: path + "." + key, Message: "Expected array."})
				continue
			}
			for i, item := range arr {
				errs = append(errs, compareStructure(star, item, path+"."+key+"."+strconv.Itoa(i))...)
			}
			continue
		}

		if vm != nil {
			child, present := lookup(am, aArr, key)
			if !present {
				errs = append(errs, Deduction{Field: path + "." + key, Message: "Missing field."})
				continue
			}
			if _, childIsObj := child.(map[string]any); !childIsObj {
				errs = append(errs, Deduction{Field: path + "." + key, Message: "Expected object."})
				continue
			}
			errs = append(errs, compareStructure(value, child, path+"."+key)...)
		}
	}
	return errs
}

// entries yields (key,value) pairs. Arrays are indexed by position (numeric
// keys), objects by their string keys, matching the JS Object.entries / map.
func entries(expected any) map[string]any {
	out := map[string]any{}
	switch t := expected.(type) {
	case []any:
		for i, v := range t {
			out[strconv.Itoa(i)] = v
		}
	case map[string]any:
		return t
	}
	return out
}

// lookup fetches key from whichever container actual turned out to be.
func lookup(m map[string]any, arr []any, key string) (any, bool) {
	if m != nil {
		v, ok := m[key]
		return v, ok
	}
	if numericKey.MatchString(key) {
		if i, err := strconv.Atoi(key); err == nil && i >= 0 && i < len(arr) {
			return arr[i], true
		}
	}
	return nil, false
}
