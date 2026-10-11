// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"strings"
	"testing"
)

// The format checkers, case by case: each row is a value and whether the
// format's grammar accepts it.
func TestFormatCheckers(t *testing.T) {
	long := func(n int, s string) string { return strings.Repeat(s, n) }
	for _, c := range []struct {
		format, value string
		ok            bool
	}{
		// duration: RFC 3339 appendix A
		{"duration", "P3Y6M4DT12H30M5S", true},
		{"duration", "P2M", true},
		{"duration", "PT90S", true},
		{"duration", "P6W", true},
		{"duration", "P0003D", true},
		{"duration", "PT2H5S", false}, // hours then seconds skips minutes
		{"duration", "P1Y3D", false},
		{"duration", "P3W1D", false},
		{"duration", "P1DT", false},
		{"duration", "PT1.5S", false},
		{"duration", "p1d", false},
		{"duration", "P", false},

		// hostname: RFC 1123 labels, A-labels checked as IDNA2008
		{"hostname", "sigma.example.in", true},
		{"hostname", "Gateway-07", true},
		{"hostname", "a--b", true},
		{"hostname", "xn--mnchen-3ya.example", true}, // münchen
		{"hostname", "xn--a", false},                 // decodes to a control character
		{"hostname", "under_score", false},
		{"hostname", "-lead", false},
		{"hostname", "trail-", false},
		{"hostname", "dot.", false},
		{"hostname", "a..b", false},
		{"hostname", long(64, "h"), false},
		{"hostname", long(63, "h"), true},
		{"hostname", long(50, "abcd.") + "abc", true},   // 253 characters
		{"hostname", long(50, "abcd.") + "abcd", false}, // 254
		{"hostname", "münchen", false},

		// idn-hostname: U-labels by IDNA2008 after the lookup mapping
		{"idn-hostname", "münchen.example", true},
		{"idn-hostname", "MÜNCHEN.example", true},
		{"idn-hostname", "例え。テスト", true},
		{"idn-hostname", "\U00000301abc", false},                           // leading combining mark
		{"idn-hostname", "ab\U00000640cd", false},                          // TATWEEL is disallowed
		{"idn-hostname", "x\U000000b7y", false},                            // middle dot outside l·l
		{"idn-hostname", "col\U000000b7la", true},                          // Catalan l·l
		{"idn-hostname", "\U000005d0\U000005f3", true},                     // geresh after Hebrew
		{"idn-hostname", "a\U000005f3", false},                             // geresh after Latin
		{"idn-hostname", "\U000030fbx", false},                             // katakana middle dot without Japanese script
		{"idn-hostname", "\U000030fb\U000030ab", true},                     // with katakana
		{"idn-hostname", "\U00000661\U000006f1", false},                    // two kinds of Arabic-Indic digits
		{"idn-hostname", "\U00000915\U0000094d\U0000200d\U00000937", true}, // ZWJ after virama
		{"idn-hostname", "ab\U0000200dcd", false},                          // ZWJ elsewhere
		{"idn-hostname", "1a.\U000005d0\U000005d1", false},                 // bidi domain: label starting with a digit
		{"idn-hostname", "\U000005d0\U000005d1.example", true},
		{"idn-hostname", "xn--mnchen-3ya", true},
		{"idn-hostname", "xn--", false},
		{"idn-hostname", long(60, "ü"), false}, // A-label longer than 63

		// email: RFC 5321 mailbox
		{"email", "ops@sigma.example", true},
		{"email", "first.last+tag@sigma.example", true},
		{"email", `"two words"@sigma.example`, true},
		{"email", `"esc\"aped"@sigma.example`, true},
		{"email", "ops@[192.0.2.1]", true},
		{"email", "ops@[IPv6:2001:db8::1]", true},
		{"email", "ops@[192.0.2.256]", false},
		{"email", "ops@[tag:value]", false},
		{"email", "first..last@sigma.example", false},
		{"email", ".first@sigma.example", false},
		{"email", "ops@sigma..example", false},
		{"email", "ops@-sigma.example", false},
		{"email", "ops sigma@sigma.example", false},
		{"email", "Ops <ops@sigma.example>", false},
		{"email", "ops@sigma.example.", false},
		{"email", "opś@sigma.example", false},
		{"email", "ops@", false},
		{"email", long(65, "a") + "@sigma.example", false},

		// idn-email: RFC 6531
		{"idn-email", "opś@sigma.example", true},
		{"idn-email", "ops@münchen.example", true},
		{"idn-email", `"ś p"@sigma.example`, true},
		{"idn-email", "ops@abـcd.example", false},
		{"idn-email", "ops＠sigma.example", false},

		// uri, uri-reference: RFC 3986
		{"uri", "https://user@sigma.example:8443/a/b?c=d#e", true},
		{"uri", "urn:oid:1.3.6.1", true},
		{"uri", "mailto:ops@sigma.example", true},
		{"uri", "http://[2001:db8::1]/", true},
		{"uri", "http://[v7.x:y]/", true},
		{"uri", "file:///etc/hosts", true},
		{"uri", "http://sigma.example/%7Euser", true},
		{"uri", "/relative/path", false},
		{"uri", "http://sigma.example/a b", false},
		{"uri", "http://sigma.example/%zz", false},
		{"uri", "http://[2001:db8::1/", false},
		{"uri", "http://sigma.example:80a/", false},
		{"uri", "1http://x", false},
		{"uri", "http://sigma.example/ü", false},
		{"uri", "http://a/b\\c", false},
		{"uri", "http://a/#b#c", false},
		{"uri-reference", "../a/b?c#d", true},
		{"uri-reference", "", true},
		{"uri-reference", "#frag", true},
		{"uri-reference", "//host/p", true},
		{"uri-reference", "a:b", true},
		{"uri-reference", "\\\\WINDOWS\\share", false},
		{"uri-reference", "#a#b", false},
		{"uri-reference", "ü", false},

		// iri, iri-reference: RFC 3987
		{"iri", "https://münchen.example/straße?q=ä#ö", true},
		{"iri", "http://[2001:db8::1]/", true},
		{"iri", "/relative", false},
		{"iri", "http://a/b c", false},
		{"iri-reference", "straße/ä", true},
		{"iri-reference", "#ä", true},
		{"iri-reference", "\\a", false},

		// json-pointer, relative-json-pointer: RFC 6901 and the
		// relative JSON pointer draft
		{"json-pointer", "", true},
		{"json-pointer", "/a~0b/c~1d/0", true},
		{"json-pointer", "/", true},
		{"json-pointer", "a/b", false},
		{"json-pointer", "/a~2", false},
		{"json-pointer", "/a~", false},
		{"relative-json-pointer", "0", true},
		{"relative-json-pointer", "1/a/b", true},
		{"relative-json-pointer", "2#", true},
		{"relative-json-pointer", "0+1/x", true},
		{"relative-json-pointer", "3-2", true},
		{"relative-json-pointer", "01", false},
		{"relative-json-pointer", "-1", false},
		{"relative-json-pointer", "1#/a", false},
		{"relative-json-pointer", "/a", false},
		{"relative-json-pointer", "", false},
		{"relative-json-pointer", "0+01", false},

		// uri-template: RFC 6570 level 4
		{"uri-template", "https://sigma.example/{tenant}/devices{?limit,offset}", true},
		{"uri-template", "{+path:6}/here", true},
		{"uri-template", "{#x,y}{.list*}{/a}{;b}{&c}{var.sub}", true},
		{"uri-template", "{%41b}", true},
		{"uri-template", "no expression", false},
		{"uri-template", "{unclosed", false},
		{"uri-template", "closed}", false},
		{"uri-template", "{a:0}", false},
		{"uri-template", "{a:10000}", false},
		{"uri-template", "{}", false},
		{"uri-template", "{a..b}", false},
		{"uri-template", "{=a}", false},
		{"uri-template", "a<b", false},

		// unchanged checkers keep working
		{"date", "2026-10-11", true},
		{"uuid", "2eb8aa08-aa98-11ea-b4aa-73b441d16380", true},
		{"ipv4", "192.0.2.1", true},
	} {
		check, ok := checkers[c.format]
		if !ok {
			t.Errorf("no checker for %s", c.format)
			continue
		}
		msg := check(c.value)
		if got := msg == ""; got != c.ok {
			t.Errorf("%s %q: valid %v (%s), want %v", c.format, c.value, got, msg, c.ok)
		}
	}
}

// Through a set: the message names the value and what it is not.
func TestFormatMessage(t *testing.T) {
	s := NewSet(Options{AssertFormat: true})
	if err := s.Add("h.json", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","format":"hostname"}`)); err != nil {
		t.Fatal(err)
	}
	err := s.ValidateJSON("h.json", []byte(`"bad_host"`))
	ve, ok := err.(ValidationError)
	if !ok || len(ve) != 1 || ve[0].Keyword != "format" || ve[0].Message != "bad_host is not a host name" {
		t.Fatalf("got %#v", err)
	}
}
