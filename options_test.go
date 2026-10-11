// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"strings"
	"testing"
)

// format is an annotation by default for drafts 2019-09 and 2020-12, an
// assertion for drafts 4, 6 and 7; Options.AssertFormat asserts it in
// every draft. A 2020-12 meta-schema that requires the format-assertion
// vocabulary asserts it without the option.
//
// Covers: SIG-IPE-2226
func TestFormatAnnotationByDraft(t *testing.T) {
	metas := map[string]string{
		"draft4":       "http://json-schema.org/draft-04/schema#",
		"draft6":       "http://json-schema.org/draft-06/schema#",
		"draft7":       "http://json-schema.org/draft-07/schema#",
		"draft2019-09": "https://json-schema.org/draft/2019-09/schema",
		"draft2020-12": "https://json-schema.org/draft/2020-12/schema",
		"none":         "",
	}
	asserts := map[string]bool{"draft4": true, "draft6": true, "draft7": true, "none": true}
	for draft, meta := range metas {
		for _, assert := range []bool{false, true} {
			s := NewSet(Options{AssertFormat: assert})
			src := `{"type":"string","format":"date"}`
			if meta != "" {
				src = `{"$schema":"` + meta + `","type":"string","format":"date"}`
			}
			if err := s.Add("d.json", []byte(src)); err != nil {
				t.Fatal(err)
			}
			err := s.ValidateJSON("d.json", []byte(`"yesterday"`))
			if want := assert || asserts[draft]; (err != nil) != want {
				t.Errorf("%s, AssertFormat %v: %v, want a failure: %v", draft, assert, err, want)
			}
			if err := s.ValidateJSON("d.json", []byte(`"2026-10-11"`)); err != nil {
				t.Errorf("%s, AssertFormat %v: valid date refused: %v", draft, assert, err)
			}
		}
	}

	// A meta-schema with the format-assertion vocabulary, required (true)
	// or optional (false): this package knows the vocabulary, so both
	// assert.
	for _, required := range []string{"true", "false"} {
		s := NewSet(Options{})
		for name, src := range map[string]string{
			"http://x.example/meta": `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"http://x.example/meta",
			"$vocabulary":{"https://json-schema.org/draft/2020-12/vocab/core":true,"https://json-schema.org/draft/2020-12/vocab/format-assertion":` + required + `},
			"$dynamicAnchor":"meta"}`,
			"f.json": `{"$schema":"http://x.example/meta","format":"ipv4"}`,
		} {
			if err := s.Add(name, []byte(src)); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.ValidateJSON("f.json", []byte(`"1.2.3"`)); err == nil {
			t.Errorf("format-assertion vocabulary %s: an invalid ipv4 was accepted", required)
		}
		if err := s.ValidateJSON("f.json", []byte(`"192.0.2.1"`)); err != nil {
			t.Errorf("format-assertion vocabulary %s: %v", required, err)
		}
	}

	// A dialect alone does not change format: asserted in a draft-06
	// document.
	d := NewSet(Options{Dialect: OCA()})
	if err := d.Add("o.json", []byte(`{"$schema":"http://json-schema.org/draft-06/schema#","type":"string","format":"date-time"}`)); err != nil {
		t.Fatal(err)
	}
	if err := d.ValidateJSON("o.json", []byte(`"soon"`)); err == nil || !strings.Contains(err.Error(), "date-time") {
		t.Errorf("draft-06 date-time: %v", err)
	}
}

// Options.Dialect is applied by CheckSchemas.
func TestOptionsDialect(t *testing.T) {
	s := NewSet(Options{Dialect: OCA(), AssertFormat: true})
	if err := s.Add("b.json", []byte(`{"definitions":{"X":{"vendorKeyword":1}}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckSchemas("b.json", ""); err == nil {
		t.Error("a keyword outside the OCA dialect was accepted")
	}
}

// Options.AssertContent checks contentEncoding and contentMediaType;
// without it they are annotations.
func TestAssertContent(t *testing.T) {
	draft7 := `{"$schema":"http://json-schema.org/draft-07/schema#","contentEncoding":"base64","contentMediaType":"application/json"}`
	d2020 := `{"$schema":"https://json-schema.org/draft/2020-12/schema","contentEncoding":"base64","contentMediaType":"application/json",
		"contentSchema":{"required":["a"]}}`
	for _, c := range []struct {
		src    string
		assert bool
		doc    string
		want   string // "" when valid
	}{
		{draft7, false, `"not base64!"`, ""},
		{draft7, true, `"not base64!"`, "/: is not base64 (contentEncoding)"},
		{draft7, true, `"eyJhIjoxfQ=="`, ""},                                                   // {"a":1}
		{draft7, true, `"bm90IGpzb24="`, "/: is not application/json (contentMediaType)"},      // "not json"
		{d2020, true, `"eyJiIjoxfQ=="`, "/: the decoded content does not match contentSchema"}, // {"b":1}
		{d2020, true, `"eyJhIjoxfQ=="`, ""},
		{d2020, false, `"eyJiIjoxfQ=="`, ""},
	} {
		s := NewSet(Options{AssertContent: c.assert})
		if err := s.Add("c.json", []byte(c.src)); err != nil {
			t.Fatal(err)
		}
		err := s.ValidateJSON("c.json", []byte(c.doc))
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("AssertContent %v, %s: %q, want %q", c.assert, c.doc, got, c.want)
		}
		if ve, isVE := err.(ValidationError); err != nil && (!isVE || !strings.HasPrefix(ve[0].Keyword, "content")) {
			t.Errorf("AssertContent %v, %s: keyword of %v", c.assert, c.doc, err)
		}
	}
}
