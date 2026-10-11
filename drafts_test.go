// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"strings"
	"testing"
)

func setOf(t *testing.T, o Options, docs ...string) *Set {
	t.Helper()
	s := NewSet(o)
	for i := 0; i+1 < len(docs); i += 2 {
		if err := s.Add(docs[i], []byte(docs[i+1])); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// DefaultDraft chooses the draft of documents without $schema; an unknown
// URL fails at compile.
//
// Covers: SIG-IPE-2226
func TestDefaultDraft(t *testing.T) {
	// prefixItems exists in 2020-12 only; draft-07 ignores it
	const sch = `{"prefixItems":[{"type":"string"}]}`
	if err := setOf(t, Options{}, "a.json", sch).ValidateJSON("a.json", []byte(`[1]`)); err != nil {
		t.Fatalf("draft-07 default: %v", err)
	}
	s := setOf(t, Options{DefaultDraft: "https://json-schema.org/draft/2020-12/schema"}, "a.json", sch)
	if err := s.ValidateJSON("a.json", []byte(`[1]`)); err == nil {
		t.Fatal("2020-12 default: prefixItems not applied")
	}
	s = setOf(t, Options{DefaultDraft: "http://example.com/draft-99"}, "a.json", sch)
	if err := s.Compile("a.json"); err == nil || !strings.Contains(err.Error(), "default draft") {
		t.Fatalf("unknown default draft: %v", err)
	}
}

// Draft4Integer: in draft-04 documents 1.0 is not an integer; other
// drafts, number types and fractions are unaffected, and enum values are
// not marked.
//
// Covers: SIG-IPE-2226
func TestDraft4Integer(t *testing.T) {
	const d4 = `{"$schema":"http://json-schema.org/draft-04/schema#","properties":{
		"i":{"type":"integer"},"n":{"type":["integer","number"]},"e":{"enum":[{"type":"integer"}]}}}`
	const d7 = `{"$schema":"http://json-schema.org/draft-07/schema#","properties":{"i":{"type":"integer"}}}`
	for _, strict := range []bool{false, true} {
		s := setOf(t, Options{Draft4Integer: strict}, "d4.json", d4, "d7.json", d7)
		err := s.ValidateJSON("d4.json", []byte(`{"i":1.0}`))
		if (err != nil) != strict {
			t.Errorf("strict %v: 1.0 as draft-04 integer: %v", strict, err)
		}
		if strict && !hasErr(err, "/i", "must be integer, not number") {
			t.Errorf("message: %v", err)
		}
		for _, ok := range []string{`{"i":1}`, `{"n":1.0}`, `{"i":-0}`, `{"e":{"type":"integer"}}`} {
			if err := s.ValidateJSON("d4.json", []byte(ok)); err != nil {
				t.Errorf("strict %v: %s refused: %v", strict, ok, err)
			}
		}
		if err := s.ValidateJSON("d4.json", []byte(`{"i":1.5}`)); err == nil || strings.Count(err.Error(), "integer") != 1 {
			t.Errorf("strict %v: 1.5 must fail once: %v", strict, err)
		}
		if err := s.ValidateJSON("d7.json", []byte(`{"i":1.0}`)); err != nil {
			t.Errorf("strict %v: draft-07 refused 1.0: %v", strict, err)
		}
	}
	// a document without $schema read as draft-04 by DefaultDraft
	s := setOf(t, Options{Draft4Integer: true, DefaultDraft: "http://json-schema.org/draft-04/schema#"}, "x.json", `{"type":"integer"}`)
	if err := s.ValidateJSON("x.json", []byte(`2e0`)); err == nil {
		t.Error("2e0 accepted as draft-04 integer")
	}
}

// ValidateGo validates a Go value as its JSON encoding.
//
// Covers: SIG-IPE-2226
func TestValidateGo(t *testing.T) {
	s := setOf(t, Options{}, "m.json", `{"type":"object","required":["kWh"],"properties":{"kWh":{"type":"number","minimum":0}}}`)
	type reading struct {
		KWh float64 `json:"kWh"`
	}
	if err := s.ValidateGo("m.json", reading{KWh: 2.5}); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateGo("m.json", reading{KWh: -1}); !hasErr(err, "/kWh", ">= 0") {
		t.Fatalf("got %v", err)
	}
	if err := s.ValidateGo("m.json", map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("unencodable value accepted")
	}
}

// $dynamicRef across a family: a generic document's dynamic anchor is
// overridden by the document that refers to it, documents being found by
// $id and by file name; a $dynamicRef whose first target has no matching
// $dynamicAnchor behaves as $ref; 2019-09's $recursiveRef likewise.
//
// Covers: SIG-IPE-2226
func TestDynamicRefFamily(t *testing.T) {
	const tree = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://x.example/tree",
		"$dynamicAnchor":"node","type":"object",
		"properties":{"data":true,"children":{"type":"array","items":{"$dynamicRef":"#node"}}}}`
	const strict = `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://x.example/strict-tree",
		"$dynamicAnchor":"node","$ref":"tree","unevaluatedProperties":false}`
	// the generic tree is found by file name: the strict tree's $ref names "tree"
	s := setOf(t, Options{}, "tree", tree, "strict-tree.json", strict)
	bad := `{"children":[{"daat":1}]}`
	if err := s.ValidateJSON("tree", []byte(bad)); err != nil {
		t.Fatalf("generic tree refused: %v", err)
	}
	if err := s.ValidateJSON("strict-tree.json", []byte(bad)); err == nil {
		t.Fatal("strict tree accepted a misspelt member deep in the tree")
	}
	if err := s.ValidateJSON("strict-tree.json", []byte(`{"children":[{"data":1}]}`)); err != nil {
		t.Fatal(err)
	}

	// no matching $dynamicAnchor at the first target: a plain $ref
	s = setOf(t, Options{}, "a.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{"s":{"$anchor":"s","type":"string"}},"$dynamicRef":"#s"}`)
	if err := s.ValidateJSON("a.json", []byte(`1`)); err == nil {
		t.Fatal("$dynamicRef to an $anchor not applied")
	}

	// 2019-09 $recursiveRef
	s = setOf(t, Options{},
		"base.json", `{"$schema":"https://json-schema.org/draft/2019-09/schema","$id":"https://x.example/base","$recursiveAnchor":true,
			"type":"object","additionalProperties":{"$recursiveRef":"#"}}`,
		"ints.json", `{"$schema":"https://json-schema.org/draft/2019-09/schema","$id":"https://x.example/ints","$recursiveAnchor":true,
			"$ref":"base","properties":{"n":{"type":"integer"}}}`)
	if err := s.ValidateJSON("ints.json", []byte(`{"a":{"n":"x"}}`)); err == nil {
		t.Fatal("$recursiveRef did not reach the extending schema")
	}

	// CheckSchemas follows the same references without refusing them
	for _, n := range []string{"base.json", "ints.json"} {
		if _, err := s.CheckSchemas(n, ""); err != nil {
			t.Errorf("CheckSchemas %s: %v", n, err)
		}
	}
}

// Vocabularies: a meta-schema in the set that requires a vocabulary
// nobody implements is refused at compile; one that lists it as optional
// is used; a meta-schema without the validation vocabulary turns its
// keywords off.
//
// Covers: SIG-IPE-2226
func TestVocabularies(t *testing.T) {
	meta := func(id, vocab string) string {
		return `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"` + id + `","$dynamicAnchor":"meta",
			"$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/core":true,` + vocab + `}}`
	}
	s := setOf(t, Options{},
		"https://x.example/m1", meta("https://x.example/m1", `"https://x.example/vocab/unknown":true`),
		"a.json", `{"$schema":"https://x.example/m1","minimum":1}`)
	if err := s.Compile("a.json"); err == nil {
		t.Fatal("required unknown vocabulary accepted")
	}

	s = setOf(t, Options{},
		"https://x.example/m2", meta("https://x.example/m2", `"https://x.example/vocab/unknown":false,"https://json-schema.org/draft/2020-12/vocab/validation":true`),
		"b.json", `{"$schema":"https://x.example/m2","minimum":1}`)
	if err := s.ValidateJSON("b.json", []byte(`0`)); err == nil {
		t.Fatal("validation vocabulary not applied")
	}

	s = setOf(t, Options{},
		"https://x.example/m3", meta("https://x.example/m3", `"https://json-schema.org/draft/2020-12/vocab/applicator":true`),
		"c.json", `{"$schema":"https://x.example/m3","minimum":1}`)
	if err := s.ValidateJSON("c.json", []byte(`0`)); err != nil {
		t.Fatalf("minimum applied without the validation vocabulary: %v", err)
	}
	// CheckSchemas reads c.json as 2020-12 through its meta-schema
	if _, err := s.CheckSchemas("c.json", ""); err != nil {
		t.Fatal(err)
	}
}

// CheckSchemas in 2019-09 and 2020-12: $defs, anchors, embedded resources
// with their own $id, and siblings of $ref (which these drafts apply).
//
// Covers: SIG-IPE-2363
func TestCheckSchemasModern(t *testing.T) {
	s := setOf(t, Options{}, "a.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://x.example/a",
		"$ref":"#/$defs/loop","minLength":1,
		"$defs":{"item":{"$id":"item","$defs":{"x":{"$anchor":"x","type":"string"}},"$ref":"#x"},
		         "loop":{"properties":{"i":{"$ref":"item"},"j":{"$ref":"https://x.example/item#/$defs/x"},"k":{"$dynamicRef":"#x2"}}},
		         "x2":{"$dynamicAnchor":"x2"}}}`)
	if _, err := s.CheckSchemas("a.json", ""); err != nil {
		t.Fatalf("CheckSchemas: %v", err)
	}
	// a 2020-12 keyword in a draft-07 document is refused
	s = setOf(t, Options{}, "b.json", `{"$schema":"http://json-schema.org/draft-07/schema#","$defs":{}}`)
	if _, err := s.CheckSchemas("b.json", ""); err == nil {
		t.Fatal("$defs accepted in draft-07")
	}
	// a loop through $ref beside other keywords is still found
	s = setOf(t, Options{}, "c.json", `{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"#","type":"object"}`)
	if _, err := s.CheckSchemas("c.json", ""); err == nil || !strings.Contains(err.Error(), "refers back") {
		t.Fatalf("loop: %v", err)
	}
}
