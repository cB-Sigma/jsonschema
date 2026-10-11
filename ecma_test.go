// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"strings"
	"testing"
)

// bs is a backslash, to spell ECMA-262's four-digit Unicode escapes in
// the patterns below.
const bs = `\`

// Patterns are ECMA-262 regular expressions with the u flag: what the
// syntax accepts and refuses.
func TestRegexpSyntax(t *testing.T) {
	for _, c := range []struct {
		src string
		ok  bool
	}{
		{`^[a-z]+(?:-[a-z0-9]+)*$`, true},
		{`\cJ\x41` + bs + `u0042` + bs + `u{1F432}\0`, true},
		{`[]`, true},
		{`[^]`, true},
		{`(?<year>\d{4})-\k<year>`, true},
		{`(a)\1`, true},
		{`(?<=\$)\d+`, true},
		{`(?<!a+)b`, true},
		{`x(?=y)`, true},
		{`x(?!y)`, true},
		{`\p{Lu}\P{L}\p{Script=Greek}\p{gc=Nd}\p{Letter}\p{White_Space}`, true},
		{`[\d\-\w]`, true},
		{`(?i:abc)d`, true},
		{`(?-i:a)`, true},
		{`a{2}b{2,}c{2,5}?`, true},
		{`\/`, true},
		{``, true},
		{`(?i)abc`, false},
		{`(?P<n>x)`, false},
		{`(?#c)a`, false},
		{`\a`, false},
		{`\e`, false},
		{`*a`, false},
		{`a**`, false},
		{`[z-a]`, false},
		{`[\d-z]`, false},
		{`\`, false},
		{`a)`, false},
		{`(a`, false},
		{`[abc`, false},
		{`a{`, false},
		{`a{2,1}`, false},
		{`}`, false},
		{`]`, false},
		{`(a)\2`, false},
		{`\k<nope>`, false},
		{`(?<n>a)(?<n>b)`, false},
		{`(?=a)*`, false},
		{`\p{NoSuchProperty}`, false},
		{`\u{110000}`, false},
		{`\x4`, false},
		{`\c1`, false},
	} {
		_, err := CompileRegexp(c.src)
		if (err == nil) != c.ok {
			t.Errorf("%q: %v, want ok %v", c.src, err, c.ok)
		}
	}
}

// What patterns match, by ECMA-262 rules where Go's regexp differs.
func TestRegexpMatch(t *testing.T) {
	for _, c := range []struct {
		src, in string
		want    bool
	}{
		{`^\s$`, "\U00002003", true},
		{`^\s$`, "\U0000feff", true},
		{`^\s$`, "\v", true},
		{`^\s$`, "\U00002029", true},
		{`^\S$`, "\U000000a0", false},
		{`^\d$`, "\U00000966", false},
		{`^\w$`, "é", false},
		{`^\cC$`, "\x03", true},
		{`^\cc$`, "\x03", true},
		{`^abc$`, "abc\n", false},
		{`^.$`, "\r", false},
		{`^.$`, "\U00002028", false},
		{`^.$`, "🐲", true},
		{`^🐲*$`, "🐲🐲", true},
		{`^🐲*$`, "🐉", false},
		{`[]`, "anything", false},
		{`^` + bs + `uD83D` + bs + `uDC32$`, "\U0001F432", true}, // a surrogate pair is one code point
		{`^[` + bs + `u0041-` + bs + `u005A]+$`, "AZ", true},
		{`^[^]$`, "\n", true},
		{`^\p{Letter}+$`, "école", true},
		{`^\p{digit}+$`, "৪২", true},
		{`^\p{Script=Greek}+$`, "αβγ", true},
		{`^\p{Script=Greek}+$`, "abc", false},
		{`(?i:straße)`, "STRASSE", false}, // simple case folding only
		{`^(?i:ABC)$`, "abc", true},
		{`^(?i:[a-c]+)$`, "ABC", true},
		{`colou?r`, "my colour", true},
		{`\bend\b`, "the end.", true},
		// lookaround and back references
		{`^(\w+) \1$`, "bye bye", true},
		{`^(\w+) \1$`, "bye hi", false},
		{`^(?<w>a+)-\k<w>$`, "aa-aa", true},
		{`^(?<w>a+)-\k<w>$`, "aa-a", false},
		{`^\d+(?=px)`, "12px", true},
		{`^\d+(?=px)`, "12em", false},
		{`^(?!.*--)[a-z-]+$`, "a-b", true},
		{`^(?!.*--)[a-z-]+$`, "a--b", false},
		{`(?<=\$)\d+`, "cost $42", true},
		{`(?<=\$)\d+`, "cost 42", false},
		{`(?<!a+)b`, "aab", false},
		{`(?<!a+)b`, "cb", true},
		{`(?<=(\d+)-)\1`, "12-12", true},
		{`^(?:(a)|b)\1$`, "b", true}, // a back reference to an unset group matches empty
		{`^(a*)*$`, "aaaa", true},
		{`^(?:a|ab)(?:c|bcd)(?=d)`, "abcd", true},
		{`^(?:(?=(\w))\1)+$`, "abc", true},
		{`^a{1001}$`, strings.Repeat("a", 1001), true},
		{`^a{1001}$`, strings.Repeat("a", 1000), false},
		{`(?m:^b$)`, "a\rb\rc", true},
	} {
		re, err := CompileRegexp(c.src)
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if got := re.MatchString(c.in); got != c.want {
			t.Errorf("%q on %q: %v, want %v", c.src, c.in, got, c.want)
		}
		if re.String() != c.src {
			t.Errorf("String() = %q, want %q", re.String(), c.src)
		}
	}
}

// Every pattern that is translated to RE2 matches exactly as the
// backtracking matcher does, over a corpus of inputs.
func TestRegexpPathsAgree(t *testing.T) {
	patterns := []string{
		`^\s+$`, `\S`, `^.$`, `(?s:^.$)`, `^[^]$`, `[]`, `^\w+\b`, `\B\d`, `^[\d\-a-f]+$`, `[^\s\d]`,
		`^\p{L}+$`, `\P{Lu}`, `^(?i:[a-c]+)$`, `(?i:k)`, `^(?i:[^k])$`, `colou?r`, `a{2,3}?b`, `(a|bc)+d`,
		`^` + bs + `u{1F432}*$`, `\x41` + bs + `u0042`, `\cJ`, `^$`, `x*`, `(?:ab|a)(?:c|bcd)`, `[` + bs + `u2028-` + bs + `u2029]`, `\0`,
		`^(?:[a-z0-9-]+\.)+[a-z]{2,}$`, `(?-i:a)`, `\p{Script=Han}`, `\p{White_Space}`,
	}
	inputs := []string{"", "a", "A", "abc", "ABC", "\n", "\r", "\U00002028", "\U00002003", "\U0000feff", "a b",
		"colour", "aab", "bcd", "abcd", "\U0001F432\U0001F432", "\U0001F409", "AB", "\n\x00", "k", "K", "\U0000212a",
		"sigma.example.in", "-", "12", "\U00000966", "é", "日本", "x\tz"}
	for _, src := range patterns {
		re, err := CompileRegexp(src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if re.re == nil {
			t.Errorf("%q was not translated to RE2", src)
			continue
		}
		for _, in := range inputs {
			if a, b := re.MatchString(in), re.backtrack(in); a != b {
				t.Errorf("%q on %q: RE2 %v, backtracking %v", src, in, a, b)
			}
		}
	}
}

// The backtracking matcher gives up after MaxSteps (no match) instead of
// running for ever on a pattern with catastrophic backtracking.
func TestRegexpStepLimit(t *testing.T) {
	re, err := CompileRegexp(`^(a+)+\1$`)
	if err != nil {
		t.Fatal(err)
	}
	if re.MatchString(strings.Repeat("a", 40) + "b") {
		t.Error("matched")
	}
}

// Through a set: patterns are ECMA-262 by default, Go's syntax with
// Options.GoRegexp.
func TestPatternEngineOption(t *testing.T) {
	src := `{"type":"string","pattern":"^\\s$"}`
	ecma := NewSet()
	if err := ecma.Add("p.json", []byte(src)); err != nil {
		t.Fatal(err)
	}
	if err := ecma.ValidateJSON("p.json", []byte("\"\U00002003\"")); err != nil {
		t.Errorf("ECMA-262: EM SPACE is \\s: %v", err)
	}
	re2 := NewSetWith(Options{GoRegexp: true})
	if err := re2.Add("p.json", []byte(src)); err != nil {
		t.Fatal(err)
	}
	if err := re2.ValidateJSON("p.json", []byte("\"\U00002003\"")); err == nil {
		t.Error("Go regexp: EM SPACE matched \\s")
	}
	// A Go-only pattern compiles with GoRegexp and is refused without.
	for _, c := range []struct {
		set *Set
		ok  bool
	}{{NewSet(), false}, {NewSetWith(Options{GoRegexp: true}), true}} {
		if err := c.set.Add("g.json", []byte(`{"pattern":"(?i)abc"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := c.set.Compile("g.json"); (err == nil) != c.ok {
			t.Errorf("GoRegexp %v: (?i)abc compiles: %v", c.ok, err)
		}
	}
}
