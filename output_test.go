// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"encoding/json"
	"strings"
	"testing"
)

func outputOf(t *testing.T, schema, data string, f OutputFormat, files ...string) OutputUnit {
	t.Helper()
	s := NewSet()
	if err := s.Add("s.json", []byte(schema)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(files); i += 2 {
		if err := s.Add(files[i], []byte(files[i+1])); err != nil {
			t.Fatal(err)
		}
	}
	inst, err := Decode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Output("s.json", inst, f)
	if err != nil {
		t.Fatal(err)
	}
	if (s.Validate("s.json", inst) == nil) != out.Valid {
		t.Fatalf("Output valid %v differs from Validate", out.Valid)
	}
	return out
}

func asJSON(t *testing.T, u OutputUnit) map[string]any {
	t.Helper()
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// The flag format is the result alone.
//
// Covers: SIG-IPE-2226
func TestOutputFlag(t *testing.T) {
	out := outputOf(t, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string"}`, `1`, OutputFlag)
	b, _ := json.Marshal(out)
	if string(b) != `{"valid":false}` {
		t.Fatalf("flag = %s", b)
	}
	out = outputOf(t, `{"type":"string"}`, `"a"`, OutputFlag)
	b, _ = json.Marshal(out)
	if string(b) != `{"valid":true}` {
		t.Fatalf("flag = %s", b)
	}
}

// basic lists the failures with keyword, absolute keyword and instance
// locations (pointers escaped), and drops annotations of a failed
// instance; for a valid instance it lists the annotations.
//
// Covers: SIG-IPE-2226
func TestOutputBasic(t *testing.T) {
	const id = "https://example.com/escape"
	out := outputOf(t, `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"`+id+`",
		"properties":{"~a/b":{"type":"number"}},"title":"T"}`, `{"~a/b":"x"}`, OutputBasic)
	if out.Valid || len(out.Annotations) != 0 {
		t.Fatalf("basic = %s", out)
	}
	found := false
	for _, e := range out.Errors {
		if e.KeywordLocation == "/properties/~0a~1b/type" && e.InstanceLocation == "/~0a~1b" &&
			e.AbsoluteKeywordLocation == id+"#/properties/~0a~1b/type" && e.Error == "must be number, not string" {
			found = true
		}
		if e.Annotation != nil {
			t.Errorf("failed output carries an annotation: %s", e)
		}
	}
	if !found {
		t.Fatalf("basic = %s", out)
	}

	out = outputOf(t, `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"`+id+`","readOnly":true,
		"properties":{"a":{"title":"A"}}}`, `{"a":1}`, OutputBasic)
	m := asJSON(t, out)
	if m["valid"] != true || m["errors"] != nil {
		t.Fatalf("basic = %s", out)
	}
	want := map[string]string{"/readOnly": "true", "/properties": `["a"]`, "/properties/a/title": `"A"`}
	for _, a := range out.Annotations {
		if v, ok := want[a.KeywordLocation]; ok && string(a.Annotation) == v {
			delete(want, a.KeywordLocation)
		}
	}
	if len(want) != 0 {
		t.Fatalf("annotations missing %v in %s", want, out)
	}
}

// detailed follows the schema and keeps only failing branches; verbose
// keeps every subschema, valid ones included.
//
// Covers: SIG-IPE-2226
func TestOutputDetailedVerbose(t *testing.T) {
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema",
		"$defs":{"pos":{"type":"integer","minimum":1}},
		"properties":{"a":{"$ref":"#/$defs/pos"},"b":{"type":"string"}},
		"anyOf":[{"required":["a"]},{"required":["c"]}]}`
	d := outputOf(t, schema, `{"a":0,"b":"x"}`, OutputDetailed)
	if d.Valid {
		t.Fatal("valid")
	}
	s := d.String()
	if !strings.Contains(s, `"keywordLocation": "/properties/a/$ref/minimum"`) || strings.Contains(s, "/properties/b") {
		t.Fatalf("detailed = %s", s)
	}
	if !strings.Contains(s, `"absoluteKeywordLocation": "mem:///s.json#/$defs/pos/minimum"`) {
		t.Fatalf("detailed absolute location: %s", s)
	}

	v := outputOf(t, schema, `{"a":0,"b":"x"}`, OutputVerbose)
	vs := v.String()
	for _, want := range []string{`"keywordLocation": "/properties/b"`, `"keywordLocation": "/anyOf/1"`, `"keywordLocation": "/properties/a/$ref/minimum"`} {
		if !strings.Contains(vs, want) {
			t.Errorf("verbose lacks %s:\n%s", want, vs)
		}
	}
}

// unevaluatedProperties sees what in-place applicators evaluated, and
// only valid branches count.
//
// Covers: SIG-IPE-2226
func TestOutputUnevaluated(t *testing.T) {
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema",
		"anyOf":[{"properties":{"a":true},"required":["a"]},{"properties":{"b":true},"required":["x"]}],
		"unevaluatedProperties":false}`
	out := outputOf(t, schema, `{"a":1,"b":2}`, OutputBasic)
	if out.Valid {
		t.Fatal("b is unevaluated: the second branch failed")
	}
	got := false
	for _, e := range out.Errors {
		if e.KeywordLocation == "/unevaluatedProperties" && e.InstanceLocation == "/b" {
			got = true
		}
	}
	if !got {
		t.Fatalf("basic = %s", out)
	}
	out = outputOf(t, schema, `{"a":1}`, OutputBasic)
	if !out.Valid {
		t.Fatalf("basic = %s", out)
	}
}
