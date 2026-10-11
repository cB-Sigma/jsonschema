// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"strings"
	"unicode/utf8"
)

// validURI checks s against the generic syntax of RFC 3986 section 3 (a
// URI with a scheme, or with reference also a relative reference, section
// 4.1). With iri, the characters RFC 3987 adds are allowed: ucschar
// wherever an unreserved character may be, and iprivate in the query.
func validURI(s string, iri, reference bool) bool {
	rest := s
	if c := strings.IndexAny(s, ":/?#"); c >= 0 && s[c] == ':' {
		if !validScheme(s[:c]) {
			return false
		}
		rest = s[c+1:]
	} else if !reference {
		return false
	}
	if f := strings.IndexByte(rest, '#'); f >= 0 {
		if !uriPart(rest[f+1:], iri, false, "/?") {
			return false
		}
		rest = rest[:f]
	}
	if q := strings.IndexByte(rest, '?'); q >= 0 {
		if !uriPart(rest[q+1:], iri, true, "/?") {
			return false
		}
		rest = rest[:q]
	}
	if strings.HasPrefix(rest, "//") {
		auth := rest[2:]
		path := ""
		if p := strings.IndexByte(auth, '/'); p >= 0 {
			auth, path = auth[:p], auth[p:]
		}
		return validAuthority(auth, iri) && uriPart(path, iri, false, "/")
	}
	return uriPart(rest, iri, false, "/")
}

// validScheme: a letter, then letters, digits, "+", "-" and ".".
func validScheme(s string) bool {
	if s == "" || !(s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	for i := 1; i < len(s); i++ {
		if c := s[i]; !isAlnum(rune(c)) && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func validAuthority(a string, iri bool) bool {
	if at := strings.LastIndexByte(a, '@'); at >= 0 {
		if !uriChars(a[:at], iri, false, ":") {
			return false
		}
		a = a[at+1:]
	}
	host, port := a, ""
	if strings.HasPrefix(a, "[") {
		end := strings.IndexByte(a, ']')
		if end < 0 || !validIPLiteral(a[1:end]) {
			return false
		}
		host, port = "", a[end+1:]
		if port != "" {
			if port[0] != ':' {
				return false
			}
			port = port[1:]
		}
	} else if c := strings.IndexByte(a, ':'); c >= 0 {
		host, port = a[:c], a[c+1:]
	}
	for i := 0; i < len(port); i++ {
		if port[i] < '0' || port[i] > '9' {
			return false
		}
	}
	return uriChars(host, iri, false, "")
}

// validIPLiteral: an IPv6 address or an IPvFuture between the brackets.
func validIPLiteral(s string) bool {
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		dot := strings.IndexByte(s, '.')
		if dot < 2 || dot == len(s)-1 {
			return false
		}
		for i := 1; i < dot; i++ {
			if !isHex(s[i]) {
				return false
			}
		}
		return uriChars(s[dot+1:], false, false, ":")
	}
	return validIPv6(s)
}

// uriPart checks a path, query or fragment: pchars (unreserved,
// percent-encodings, sub-delims, ":" and "@") and the extra characters.
func uriPart(s string, iri, query bool, extra string) bool {
	return uriChars(s, iri, query, ":@"+extra)
}

// uriChars accepts unreserved characters, percent-encodings, sub-delims
// and the extra ASCII characters; with iri also ucschar, and iprivate in
// a query.
func uriChars(s string, iri, query bool, extra string) bool {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '%':
			if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
				return false
			}
			n = 3
		case r < 0x80:
			if !isAlnum(r) && !strings.ContainsRune("-._~!$&'()*+,;=", r) && !strings.ContainsRune(extra, r) {
				return false
			}
		case !iri:
			return false
		case ucschar(r):
		case query && iprivate(r):
		default:
			return false
		}
		i += n
	}
	return true
}

// ucschar is the set of non-ASCII characters RFC 3987 allows in IRIs.
func ucschar(r rune) bool {
	switch {
	case r >= 0xA0 && r <= 0xD7FF, r >= 0xF900 && r <= 0xFDCF, r >= 0xFDF0 && r <= 0xFFEF:
		return true
	case r >= 0x10000 && r <= 0xEFFFD:
		return r&0xFFFF <= 0xFFFD && !(r >= 0xE0000 && r < 0xE1000)
	}
	return false
}

// iprivate is the set of private-use characters RFC 3987 allows in the
// query of an IRI.
func iprivate(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF || r >= 0xF0000 && r <= 0xFFFFD || r >= 0x100000 && r <= 0x10FFFD
}
