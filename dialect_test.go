package jsonschema

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
)

// hasErr reports whether err lists an error at path whose message contains
// frag.
func hasErr(err error, path, frag string) bool {
	var ve ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, e := range ve {
		if e.Path == path && strings.Contains(e.Message, frag) {
			return true
		}
	}
	return false
}

func intOf(v any) (int, bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(fmtAny(v)))
	if !ok || !r.IsInt() {
		return 0, false
	}
	return int(r.Num().Int64()), true
}

func fmtAny(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// CheckSchemas walks schemas before validation: the keyword allow-list,
// each keyword's Check with pointers into the document, boolean schemas
// when the dialect refuses them, unresolved references, and references
// that loop without passing through a value.
//
// Covers: SIG-IPE-2363
func TestDialectCheckSchemas(t *testing.T) {
	d := &Dialect{
		Keywords: map[string]Keyword{
			"minLength": {Check: func(c *CheckContext, _ map[string]any, v any) {
				if n, ok := intOf(v); !ok || n < 0 {
					c.Fail("must be a non-negative integer")
				}
			}},
			"x-ref": {Check: func(c *CheckContext, _ map[string]any, v any) {
				c.FailAt("/elsewhere", "x-ref %v checked", v)
			}},
		},
		Allowed:           func(k string) bool { return k == "type" || k == "properties" || k == "allOf" || k == "items" },
		ObjectSchemasOnly: true,
	}
	s := NewSet(Options{Dialect: d})
	if err := s.Add("doc.json", []byte(`{
		"good": {"type": "object", "properties": {"a": {"type": "string", "minLength": 1}, "b": {"$ref": "#/good/properties/a"}}},
		"bad": {"type": "string", "minLength": -1, "pattern": "x", "x-ref": 1},
		"boolean": {"items": true},
		"missing": {"$ref": "#/nope"},
		"loopA": {"$ref": "#/loopB"},
		"loopB": {"allOf": [{"$ref": "#/loopA"}]},
		"valueLoop": {"type": "object", "properties": {"next": {"$ref": "#/valueLoop"}}}
	}`)); err != nil {
		t.Fatal(err)
	}
	visited, err := s.CheckSchemas("doc.json", "/good", "/valueLoop")
	if err != nil {
		t.Errorf("good schemas refused: %v", err)
	}
	if strings.Join(visited, " ") != "/good /good/properties/a /good/properties/b /valueLoop /valueLoop/properties/next" {
		t.Errorf("visited = %v", visited)
	}
	for _, c := range []struct{ root, path, msg string }{
		{"/bad", "/bad/minLength", "non-negative"},
		{"/bad", "/bad/pattern", `keyword "pattern" is not supported`},
		{"/bad", "/elsewhere", "x-ref 1 checked"},
		{"/boolean", "/boolean/items", "must be an object"},
		{"/missing", "/missing/$ref", "not found"},
		{"/loopA", "/loopB/allOf/0", "refers back to itself"},
		{"/nowhere", "/nowhere", "not found"},
	} {
		if _, err := s.CheckSchemas("doc.json", c.root); !hasErr(err, c.path, c.msg) {
			t.Errorf("%s: got %v, want %q at %q", c.root, err, c.msg, c.path)
		}
	}

	// without a dialect, CheckSchemas uses the draft-07 keyword list and
	// accepts boolean schemas
	plain := NewSet(Options{})
	if err := plain.Add("p.json", []byte(`{"definitions":{"a":{"items":true,"if":{"const":1},"then":{"minimum":1}},"b":{"nullable":true}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := plain.CheckSchemas("p.json", "/definitions/a"); err != nil {
		t.Errorf("draft-07 schema refused: %v", err)
	}
	if _, err := plain.CheckSchemas("p.json", "/definitions/b"); !hasErr(err, "/definitions/b/nullable", "not supported") {
		t.Errorf("draft-07 unknown keyword: %v", err)
	}
}
