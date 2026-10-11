// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"testing"
	"unicode/utf8"
)

// Any pattern either compiles or is refused, without a panic; a pattern
// translated to RE2 matches as the backtracking matcher does.
func FuzzRegexp(f *testing.F) {
	for _, s := range []string{`^[a-z]+$`, `(?<=a)b`, `(a)\1`, `[^]`, `\p{L}`, `(?i:ß)`, `a{2,3}`, `\cA`, `(?m:^$)`} {
		f.Add(s, "abc")
	}
	f.Fuzz(func(t *testing.T, src, in string) {
		if !utf8.ValidString(src) || !utf8.ValidString(in) || len(in) > 64 {
			return
		}
		re, err := CompileRegexp(src)
		if err != nil {
			return
		}
		got := re.MatchString(in)
		if re.re != nil {
			if b := re.backtrack(in); b != got {
				t.Fatalf("%q on %q: RE2 %v, backtracking %v", src, in, got, b)
			}
		}
	})
}

// Every format checker takes any string without a panic.
func FuzzFormats(f *testing.F) {
	for _, s := range []string{"xn--mnchen-3ya", "a@[IPv6:::1]", "{+x:3}", "0#", "P1Y", "http://[v1.x]/", "\U00003002"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, check := range checkers {
			check(s)
		}
	})
}

// Output in every format agrees with Validate on any instance, and a
// failure lists at least one error, without a panic.
func FuzzOutput(f *testing.F) {
	for _, s := range []string{`{"a":[1,"x",{"b":null}]}`, `[1,2,3]`, `"s"`, `{"c":{"c":{}}}`, `1.0`} {
		f.Add(s)
	}
	s := NewSet(Options{})
	_ = s.Add("f.json", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$defs":{"n":{"$dynamicAnchor":"n","type":["object","integer"],"properties":{"c":{"$dynamicRef":"#n"}}}},
		"anyOf":[{"$ref":"#/$defs/n"},{"type":"array","prefixItems":[{"const":1}],"contains":{"type":"string"},"unevaluatedItems":false}],
		"properties":{"a":{"items":{"not":{"type":"boolean"}}}},"unevaluatedProperties":{"type":"object"}}`))
	f.Fuzz(func(t *testing.T, data string) {
		inst, err := Decode([]byte(data))
		if err != nil {
			return
		}
		valid := s.Validate("f.json", inst) == nil
		for _, of := range []OutputFormat{OutputFlag, OutputBasic, OutputDetailed, OutputVerbose} {
			out, err := s.Output("f.json", inst, of)
			if err != nil || out.Valid != valid {
				t.Fatalf("format %d on %s: valid %v, Validate %v, %v", of, data, out.Valid, valid, err)
			}
			if of != OutputFlag && !valid && out.Error == "" && len(out.Errors) == 0 {
				t.Fatalf("format %d on %s: a failure without errors", of, data)
			}
		}
	})
}
