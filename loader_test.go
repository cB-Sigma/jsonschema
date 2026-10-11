// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import "testing"

// A document added under its full URL, without an $id, is found by that
// URL, before a document that only shares its file name; the file-name
// fallback still applies to relative names.
func TestLoaderFullURLName(t *testing.T) {
	s := NewSet(Options{})
	add := func(name, src string) {
		t.Helper()
		if err := s.Add(name, []byte(src)); err != nil {
			t.Fatal(err)
		}
	}
	add("http://localhost:1234/a/integer.json", `{"type":"integer"}`)
	add("http://localhost:1234/b/integer.json", `{"type":"string"}`)
	add("phase.json", `{"enum":["ONE","ALL"]}`)
	add("root.json", `{"properties":{
		"a":{"$ref":"http://localhost:1234/a/integer.json"},
		"b":{"$ref":"http://localhost:1234/b/integer.json"},
		"p":{"$ref":"http://x.example/s/phase.json"}}}`)

	if _, err := s.CheckSchemas("root.json", ""); err != nil {
		t.Fatalf("CheckSchemas: %v", err)
	}
	for _, c := range []struct {
		doc string
		ok  bool
	}{
		{`{"a":1,"b":"x","p":"ONE"}`, true},
		{`{"a":"x"}`, false},
		{`{"b":1}`, false},
		{`{"p":"TWO"}`, false},
	} {
		err := s.ValidateJSON("root.json", []byte(c.doc))
		if (err == nil) != c.ok {
			t.Errorf("%s: got %v, want ok=%v", c.doc, err, c.ok)
		}
	}
}
