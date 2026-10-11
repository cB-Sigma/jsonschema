package jsonschema

import (
	"strings"
	"testing"
)

// Draft-07 keywords, references across documents and JSON pointers, and
// the rule that siblings of $ref are ignored.
//
// Covers: SIG-IPE-2226
func TestValidator(t *testing.T) {
	s := NewSet()
	add := func(name, src string) {
		t.Helper()
		if err := s.Add(name, []byte(src)); err != nil {
			t.Fatal(err)
		}
	}
	add("phase.json", `{"$id":"http://x.example/s/phase.json","type":"string","enum":["ONE","ALL"]}`)
	add("item.json", `{"$id":"http://x.example/s/item_other_name.json","type":"object","properties":{"v":{"type":"number","minimum":0,"exclusiveMaximum":10}},"required":["v"]}`)
	add("root.json", `{
		"$id":"http://x.example/s/root.json","type":"object",
		"definitions":{"n":{"type":"integer","multipleOf":2}},
		"properties":{
			"phase":{"$ref":"phase.json"},
			"items":{"type":"array","items":{"$ref":"item.json"},"minItems":1,"maxItems":2,"uniqueItems":true},
			"even":{"$ref":"#/definitions/n"},
			"when":{"type":"string","format":"date-time"},
			"name":{"type":"string","minLength":2,"maxLength":4,"pattern":"^[a-z]+$"},
			"sib":{"$ref":"phase.json","type":"integer"},
			"one":{"oneOf":[{"type":"string"},{"type":"number"}]},
			"not":{"not":{"type":"null"}},
			"closed":{"type":"object","additionalProperties":false,"properties":{"a":{"const":1}}}
		},
		"required":["phase"]}`)
	ok := `{"phase":"ALL","items":[{"v":1},{"v":2.5}],"even":4,"when":"2020-01-01T00:00:00+02:00","name":"abc","sib":"ONE","one":"x","not":1,"closed":{"a":1.0}}`
	if err := s.ValidateJSON("root.json", []byte(ok)); err != nil {
		t.Fatalf("valid document refused: %v", err)
	}
	bad := map[string]string{
		"required":             `{}`,
		"enum":                 `{"phase":"TWO"}`,
		"minItems":             `{"phase":"ALL","items":[]}`,
		"maxItems":             `{"phase":"ALL","items":[{"v":1},{"v":2},{"v":3}]}`,
		"uniqueItems":          `{"phase":"ALL","items":[{"v":1},{"v":1.0}]}`,
		"minimum":              `{"phase":"ALL","items":[{"v":-1}]}`,
		"exclusiveMaximum":     `{"phase":"ALL","items":[{"v":10}]}`,
		"integer":              `{"phase":"ALL","even":2.5}`,
		"multipleOf":           `{"phase":"ALL","even":3}`,
		"format":               `{"phase":"ALL","when":"2020-01-01T00:00:00"}`,
		"pattern":              `{"phase":"ALL","name":"AB"}`,
		"maxLength":            `{"phase":"ALL","name":"abcde"}`,
		"sibling ignored":      `{"phase":"ALL","sib":1}`,
		"oneOf":                `{"phase":"ALL","one":true}`,
		"not":                  `{"phase":"ALL","not":null}`,
		"additionalProperties": `{"phase":"ALL","closed":{"b":1}}`,
		"const":                `{"phase":"ALL","closed":{"a":2}}`,
		"not JSON":             `{"phase":`,
		"trailing data":        `{"phase":"ALL"} {}`,
	}
	for what, doc := range bad {
		err := s.ValidateJSON("root.json", []byte(doc))
		if err == nil {
			t.Errorf("%s: accepted %s", what, doc)
			continue
		}
		if ve, isVE := err.(ValidationError); !isVE || len(ve) == 0 || ve[0].Message == "" {
			t.Errorf("%s: %v", what, err)
		}
	}
	// The failing path is reported.
	err := s.ValidateJSON("root.json", []byte(`{"phase":"ALL","items":[{"v":1},{"v":-2}]}`))
	if err == nil || !strings.Contains(err.Error(), "/items/1/v") {
		t.Errorf("path: %v", err)
	}
	if s.ValidateJSON("nope.json", []byte(`{}`)) == nil {
		t.Error("an unknown schema validated")
	}
}

// Covers: SIG-IPE-2550
// The OCA dialect accepts the OCPP schemas' annotations and refuses a
// keyword the validator does not implement, also nested under properties,
// items and definitions, and the draft-04 boolean exclusiveMinimum.
func TestOCADialect(t *testing.T) {
	s := NewDialectSet(OCA())
	if err := s.Add("a.json", []byte(`{"$schema":"http://json-schema.org/draft-06/schema#","id":"urn:x","javaType":"A","definitions":{"X":{"type":"string","comment":"c"}},
		"properties":{"x":{"$ref":"#/definitions/X"},"y":{"type":"array","items":{"type":"integer","multipleOf":1}}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckSchemas("a.json", ""); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"properties":{"x":{"items":{"unevaluatedItems":false}}}}`,
		`{"definitions":{"X":{"vendorKeyword":1}}}`,
		`{"properties":{"n":{"type":"number","minimum":0,"exclusiveMinimum":true}}}`,
	} {
		b := NewDialectSet(OCA())
		_ = b.Add("b.json", []byte(bad))
		if _, err := b.CheckSchemas("b.json", ""); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
	if err := s.ValidateJSON("a.json", []byte(`{"x":"s","y":[1,2]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateJSON("a.json", []byte(`{"x":1}`)); err == nil || !strings.Contains(err.Error(), "must be string") {
		t.Fatalf("%v", err)
	}
}

// What the earlier validator did on purpose and the library must keep:
// numbers compared exactly beyond float64, "integer" taking 2.0, unknown
// formats (such as the OCA's "UTF8") not checked, a reference resolved by file name when no
// document carries the $id it resolves to, and siblings of $ref ignored
// in a draft-07 document.
//
// Covers: SIG-IPE-2226
func TestDraft07Specials(t *testing.T) {
	s := NewSet()
	for name, src := range map[string]string{
		"n.json":      `{"type":"object","properties":{"big":{"type":"integer","maximum":9007199254740992},"i":{"type":"integer"},"h":{"type":"string","format":"UTF8"},"d":{"type":"string","format":"x-vendor"}}}`,
		"r.json":      `{"$id":"http://x.example/a/r.json","properties":{"t":{"$ref":"target.json"},"s":{"$ref":"target.json","maxLength":1}}}`,
		"target.json": `{"$id":"http://x.example/elsewhere/other-name.json","type":"string","minLength":2}`,
	} {
		if err := s.Add(name, []byte(src)); err != nil {
			t.Fatal(err)
		}
	}
	for doc, want := range map[string]string{
		`{"big":9007199254740993}`: "must be <= 9007199254740992",
		`{"i":2.5}`:                "must be integer, not number",
		`{"t":"x"}`:                "needs at least 2 characters",
	} {
		if err := s.ValidateJSON(map[bool]string{true: "r.json", false: "n.json"}[strings.Contains(doc, `"t"`)], []byte(doc)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", doc, err, want)
		}
	}
	for _, doc := range []string{`{"big":9007199254740992,"i":2.0,"h":"not a host!","d":"nope"}`} {
		if err := s.ValidateJSON("n.json", []byte(doc)); err != nil {
			t.Errorf("%s: %v", doc, err)
		}
	}
	if err := s.ValidateJSON("r.json", []byte(`{"s":"long"}`)); err != nil {
		t.Errorf("sibling of $ref applied: %v", err)
	}
}
