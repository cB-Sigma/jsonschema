package jsonschema

import (
	"math/big"
	"reflect"
	"testing"
)

// The library's failures are reported as pointer, keyword and our message,
// depth first by location (members by name, items by index), at one
// location in keyword order; anyOf and oneOf report once, with the closest
// branch when the style asks; propertyNames failures sit at the member,
// additionalItems at each extra item; the style words bounds, multipleOf
// and patterns and caps the list. The draft named by $schema applies
// (draft-04 boolean exclusive bounds, draft-07 siblings of $ref ignored).
//
// Covers: SIG-IPE-2362
func TestTranslate(t *testing.T) {
	s := NewSet(Options{})
	add := func(name, src string) {
		t.Helper()
		if err := s.Add(name, []byte(src)); err != nil {
			t.Fatal(err)
		}
	}
	add("o.json", `{"type":"object","required":["z","a"],"minProperties":4,
		"properties":{
			"b":{"type":"array","minItems":3,"items":[{"type":"string"}],"additionalItems":false},
			"a":{"type":["string","null"],"maxLength":2,"pattern":"^x"},
			"n":{"type":"number","minimum":1.5,"multipleOf":0.5},
			"e":{"enum":["A","B"]}
		},
		"propertyNames":{"maxLength":3},
		"additionalProperties":false}`)
	add("c.json", `{"anyOf":[{"type":"object","required":["a","b"]},{"type":"object","required":["a"]}],"not":{"required":["x"]}}`)
	add("d4.json", `{"$schema":"http://json-schema.org/draft-04/schema#","type":"number","minimum":0,"exclusiveMinimum":true}`)

	v, _ := Decode([]byte(`{"b":[1,"x","y",true],"a":7,"long":1,"n":1.2,"e":"C"}`))
	got := s.Validate("o.json", v).(ValidationError)
	exp := []Error{
		{"", "required", "z is required"},
		{"/a", "type", "must be string or null, not number"},
		{"/b/0", "type", "must be string, not number"},
		{"/b/1", "false", "no value is allowed here"},
		{"/b/2", "false", "no value is allowed here"},
		{"/b/3", "false", "no value is allowed here"},
		{"/e", "enum", `"C" is not one of ["A","B"]`},
		{"/long", "additionalProperties", "long is not an allowed member"},
		{"/long", "maxLength", "allows at most 3 characters"},
		{"/n", "minimum", "must be >= 1.50000000"},
		{"/n", "multipleOf", "must be a multiple of 1/2"},
	}
	if !reflect.DeepEqual([]Error(got), exp) {
		t.Errorf("got\n%v\nwant\n%v", got, ValidationError(exp))
	}

	// anyOf: one failure, and the closest branch on request
	cv, _ := Decode([]byte(`{"x":1}`))
	sch, err := s.compile("c.json")
	if err != nil {
		t.Fatal(err)
	}
	plain := Translate(sch.Validate(cv), cv, Style{}, nil)
	if !reflect.DeepEqual([]Error(plain), []Error{
		{"", "anyOf", "matches none of the anyOf schemas"},
		{"", "not", "must not match the not schema"},
	}) {
		t.Errorf("anyOf: %v", plain)
	}
	st := Style{ClosestBranch: true, Pattern: "does not match the pattern ",
		Bound: func(r *big.Rat) string { return r.RatString() }, Multiple: func(r *big.Rat) string { return r.FloatString(1) }}
	closest := Translate(sch.Validate(cv), cv, st, nil)
	if !reflect.DeepEqual([]Error(closest), []Error{
		{"", "anyOf", "matches none of the anyOf schemas"},
		{"", "required", "(closest branch) a is required"},
		{"", "not", "must not match the not schema"},
	}) {
		t.Errorf("closest branch: %v", closest)
	}
	osch, _ := s.compile("o.json")
	styled := Translate(osch.Validate(v), v, st, nil)
	if !hasErr(styled, "/n", "must be >= 3/2") || !hasErr(styled, "/n", "multiple of 0.5") {
		t.Errorf("style: %v", styled)
	}
	pv, _ := Decode([]byte(`{"a":"yy"}`))
	if !hasErr(Translate(osch.Validate(pv), pv, st, nil), "/a", "does not match the pattern ^x") {
		t.Errorf("pattern style")
	}
	if capped := Translate(osch.Validate(v), v, Style{Max: 2}, nil); len(capped) != 2 {
		t.Errorf("Max 2: %v", capped)
	}

	// draft-04: the boolean exclusiveMinimum makes minimum exclusive
	if err := s.ValidateJSON("d4.json", []byte(`0`)); !hasErr(err, "", "must be > 0") {
		t.Errorf("draft-04 exclusive minimum: %v", err)
	}
	if err := s.ValidateJSON("d4.json", []byte(`0.1`)); err != nil {
		t.Errorf("draft-04: %v", err)
	}
	// a set is frozen once in use
	if err := s.Add("late.json", []byte(`{}`)); err == nil {
		t.Error("a document was added after compiling")
	}
}
