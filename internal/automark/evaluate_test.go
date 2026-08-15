package automark

import "testing"

func TestCompareStructureArrayOfObjects(t *testing.T) {
	expected := map[string]any{"list": map[string]any{"*": map[string]any{"0": "id", "name": []any{"first"}}}}
	actual := map[string]any{"list": []any{
		map[string]any{"id": 1, "name": map[string]any{"first": "a"}},
		map[string]any{"name": map[string]any{"first": "b"}}, // missing id
	}}
	errs := compareStructure(expected, actual, "data")
	if len(errs) != 1 || errs[0].Field != "data.list.1.id" {
		t.Fatalf("want 1 error at data.list.1.id, got %+v", errs)
	}
}

func TestEvaluateMissingMessageAndStatus(t *testing.T) {
	a := Assertion{Score: 1, Deduction: f(0.25),
		Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success", "message": "Hello World"}}}
	// case-insensitive message match; status matches; only status_code wrong.
	r := evaluate(a, response{status: 500, json: map[string]any{"status": "success", "message": "hello world"}})
	if len(r.Deductions) != 1 || r.Deductions[0].Field != "status_code" {
		t.Fatalf("want single status_code deduction, got %+v", r.Deductions)
	}
	if r.Score != 0.75 {
		t.Fatalf("score=%v, want 0.75", r.Score)
	}
}

func TestEvaluateNetworkErrorFloorsAtZero(t *testing.T) {
	a := Assertion{Score: 1, // deduction nil => full penalty per fail
		Expected: Expected{StatusCode: 200, Body: map[string]any{"status": "success"}}}
	r := evaluate(a, response{status: 0, networkError: "timeout"})
	if r.Score != 0 || r.Passed {
		t.Fatalf("want 0 and failed, got score=%v passed=%v", r.Score, r.Passed)
	}
}

func f(v float64) *float64 { return &v }
