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

// TestBuilderEmittedShapes pins the three expected.body forms the JS builder's
// serChildren produces, proving each evaluates as the tree intends. This is the
// serializer's contract: the builder must emit exactly these shapes.
func TestBuilderEmittedShapes(t *testing.T) {
	// all-field children -> plain array. Asserts data.token and data.user exist.
	arrayForm := map[string]any{"data": []any{"token", "user"}}
	pass := map[string]any{"data": map[string]any{"token": "t", "user": "u"}}
	if errs := evalBody(arrayForm, pass); len(errs) != 0 {
		t.Fatalf("array form should pass, got %+v", errs)
	}
	if errs := evalBody(arrayForm, map[string]any{"data": map[string]any{"token": "t"}}); len(errs) != 1 || errs[0].Field != "data.user" {
		t.Fatalf("array form should flag missing data.user, got %+v", errs)
	}

	// list-of form: {"*": [...]}. errors is an array whose items each have field.
	// The star only fires when nested under a parent object key, so this is the
	// shape a top-level "object" node with a "list" child emits.
	listForm := map[string]any{"data": map[string]any{"errors": map[string]any{"*": []any{"field"}}}}
	okList := map[string]any{"data": map[string]any{"errors": []any{map[string]any{"field": "x"}, map[string]any{"field": "y"}}}}
	if errs := evalBody(listForm, okList); len(errs) != 0 {
		t.Fatalf("list form should pass, got %+v", errs)
	}
	badList := map[string]any{"data": map[string]any{"errors": []any{map[string]any{"nope": "x"}}}}
	if errs := evalBody(listForm, badList); len(errs) != 1 || errs[0].Field != "data.errors.0.field" {
		t.Fatalf("list form should flag data.errors.0.field, got %+v", errs)
	}

	// Engine limitation, pinned so the builder can honestly surface it: a
	// top-level list-of has no parent to trigger the star, so it is a no-op.
	tlList := map[string]any{"errors": map[string]any{"*": []any{"field"}}}
	if errs := evalBody(tlList, map[string]any{"errors": []any{map[string]any{"nope": "x"}}}); len(errs) != 0 {
		t.Fatalf("top-level list-of is a no-op by design, got %+v", errs)
	}

	// mixed form: numeric keys for field children + named keys for nested
	// object/list children, all in one object. serChildren emits this when a
	// level has both. Here: data.token exists, data.user exists, and data.errors
	// is a list whose items each have field.
	mixedForm := map[string]any{"data": map[string]any{"0": "token", "1": "user", "errors": map[string]any{"*": []any{"field"}}}}
	okMixed := map[string]any{"data": map[string]any{"token": "t", "user": "u", "errors": []any{map[string]any{"field": "x"}}}}
	if errs := evalBody(mixedForm, okMixed); len(errs) != 0 {
		t.Fatalf("mixed form should pass, got %+v", errs)
	}
	if errs := evalBody(mixedForm, map[string]any{"data": map[string]any{"token": "t", "errors": []any{map[string]any{"field": "x"}}}}); len(errs) != 1 || errs[0].Field != "data.user" {
		t.Fatalf("mixed form should flag missing data.user, got %+v", errs)
	}
}

// evalBody runs one expected.body against a decoded response, returning the
// non-status_code deductions (the structural ones).
func evalBody(body, json map[string]any) []Deduction {
	a := Assertion{Score: 1, Expected: Expected{StatusCode: 200, Body: body}}
	return evaluate(a, response{status: 200, json: json}).Deductions
}
