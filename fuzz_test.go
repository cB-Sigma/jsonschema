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
