// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ignoredFormats are formats the library knows that are not JSON Schema
// formats; they are not checked. ("regex" is checked by the library with
// the pattern engine and cannot be replaced.)
var ignoredFormats = []string{"period", "semver"}

var (
	dateTimeRE = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}[Tt][0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?([Zz]|[+-][0-9]{2}:[0-9]{2})$`)
	timeRE     = regexp.MustCompile(`^[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?([Zz]|[+-][0-9]{2}:[0-9]{2})$`)
	uuidRE     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

	// durationRE is the duration grammar of RFC 3339 appendix A: date
	// elements in order without gaps (year, month, day), time elements
	// likewise (hour, minute, second), or weeks alone.
	durationRE = regexp.MustCompile(`^P(?:(?:[0-9]+D|[0-9]+M(?:[0-9]+D)?|[0-9]+Y(?:[0-9]+M(?:[0-9]+D)?)?)(?:T` + durTime + `)?|T` + durTime + `|[0-9]+W)$`)
)

const durTime = `(?:[0-9]+H(?:[0-9]+M(?:[0-9]+S)?)?|[0-9]+M(?:[0-9]+S)?|[0-9]+S)`

// checkers return why a string is not in the format, or "". Values of
// other types are always in a format.
var checkers = map[string]func(s string) string{
	"date-time": func(s string) string {
		if !dateTimeRE.MatchString(s) {
			return "is not an RFC 3339 date-time with a time zone"
		}
		if _, err := time.Parse("2006-01-02", s[:10]); err != nil || !validTime(s[11:]) {
			return "is not a valid date-time"
		}
		return ""
	},
	"date": func(s string) string {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "is not an RFC 3339 full-date"
		}
		return ""
	},
	"time": func(s string) string {
		if !timeRE.MatchString(s) || !validTime(s) {
			return "is not an RFC 3339 full-time"
		}
		return ""
	},
	"duration": func(s string) string {
		if !durationRE.MatchString(s) {
			return "is not an RFC 3339 duration"
		}
		return ""
	},
	"email": func(s string) string {
		if !validEmail(s, false) {
			return "is not an e-mail address"
		}
		return ""
	},
	"idn-email": func(s string) string {
		if !validEmail(s, true) {
			return "is not an internationalised e-mail address"
		}
		return ""
	},
	"hostname": func(s string) string {
		if !validHostname(s) {
			return "is not a host name"
		}
		return ""
	},
	"idn-hostname": func(s string) string {
		if !validIDNHostname(s) {
			return "is not an internationalised host name"
		}
		return ""
	},
	"ipv4": func(s string) string {
		if !validIPv4(s) {
			return "is not an IPv4 address"
		}
		return ""
	},
	"ipv6": func(s string) string {
		if !validIPv6(s) {
			return "is not an IPv6 address"
		}
		return ""
	},
	"uri": func(s string) string {
		if !validURI(s, false, false) {
			return "is not an absolute URI"
		}
		return ""
	},
	"uri-reference": func(s string) string {
		if !validURI(s, false, true) {
			return "is not a URI reference"
		}
		return ""
	},
	"iri": func(s string) string {
		if !validURI(s, true, false) {
			return "is not an absolute IRI"
		}
		return ""
	},
	"iri-reference": func(s string) string {
		if !validURI(s, true, true) {
			return "is not an IRI reference"
		}
		return ""
	},
	"uri-template": func(s string) string {
		if !validURITemplate(s) {
			return "is not a URI template"
		}
		return ""
	},
	"json-pointer": func(s string) string {
		if !validJSONPointer(s) {
			return "is not a JSON pointer"
		}
		return ""
	},
	"relative-json-pointer": func(s string) string {
		if !validRelativeJSONPointer(s) {
			return "is not a relative JSON pointer"
		}
		return ""
	},
	"uuid": func(s string) string {
		if !uuidRE.MatchString(s) {
			return "is not a UUID"
		}
		return ""
	},
}

// validTime checks the ranges of an RFC 3339 full-time that matched timeRE
// (or the time part of a date-time): hours below 24, minutes below 60, an
// offset of at most 23:59, and second 60 only where the time is 23:59 in
// UTC (a leap second, RFC 3339 section 5.7).
func validTime(s string) bool {
	two := func(i int) int { return int(s[i]-'0')*10 + int(s[i+1]-'0') }
	h, m, sec := two(0), two(3), two(6)
	if h > 23 || m > 59 || sec > 60 {
		return false
	}
	off := 0
	if z := s[len(s)-1]; z != 'Z' && z != 'z' {
		o := s[len(s)-6:]
		oh, om := int(o[1]-'0')*10+int(o[2]-'0'), int(o[4]-'0')*10+int(o[5]-'0')
		if oh > 23 || om > 59 {
			return false
		}
		off = oh*60 + om
		if o[0] == '-' {
			off = -off
		}
	}
	if sec == 60 {
		utc := ((h*60+m-off)%1440 + 1440) % 1440
		return utc == 23*60+59
	}
	return true
}

// validIPv4 accepts the dotted-quad form of RFC 2673 section 3.2: four
// decimal octets without leading zeros.
func validIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

// validIPv6 accepts the text forms of RFC 4291 section 2.2, without a
// zone.
func validIPv6(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is6() && a.Zone() == ""
}

// validEmail accepts an RFC 5321 Mailbox (section 4.1.2): a dot-string or
// quoted-string local part of at most 64 octets, "@", and a host name or
// an IPv4 or IPv6 address literal. With idn, RFC 6531 adds non-ASCII
// characters to the local part and the domain may be an internationalised
// host name.
func validEmail(s string, idn bool) bool {
	at := strings.LastIndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if len(local) > 64 || !validLocalPart(local, idn) {
		return false
	}
	if strings.HasPrefix(domain, "[") && strings.HasSuffix(domain, "]") {
		lit := domain[1 : len(domain)-1]
		if len(lit) > 5 && strings.EqualFold(lit[:5], "IPv6:") {
			return validIPv6(lit[5:])
		}
		return validIPv4(lit)
	}
	if idn {
		return validIDNHostname(domain)
	}
	return validHostname(domain)
}

func validLocalPart(s string, idn bool) bool {
	if strings.HasPrefix(s, `"`) {
		if len(s) < 2 || !strings.HasSuffix(s, `"`) {
			return false
		}
		in := s[1 : len(s)-1]
		for i := 0; i < len(in); {
			r, n := utf8.DecodeRuneInString(in[i:])
			switch {
			case r == '\\':
				if i+1 >= len(in) || in[i+1] < 32 || in[i+1] > 126 {
					return false
				}
				n = 2
			case r == 32 || r == 33 || r >= 35 && r <= 91 || r >= 93 && r <= 126:
			case idn && r >= 0x80:
			default:
				return false
			}
			i += n
		}
		return true
	}
	for _, atom := range strings.Split(s, ".") {
		if atom == "" {
			return false
		}
		for _, r := range atom {
			if !(isAlnum(r) || strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", r) || idn && r >= 0x80) {
				return false
			}
		}
	}
	return true
}

func isAlnum(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

// validJSONPointer accepts an RFC 6901 JSON pointer.
func validJSONPointer(s string) bool {
	if s == "" {
		return true
	}
	if s[0] != '/' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '~' && (i+1 == len(s) || s[i+1] != '0' && s[i+1] != '1') {
			return false
		}
	}
	return true
}

// validRelativeJSONPointer accepts a relative JSON pointer: a
// non-negative integer without leading zeros, an optional index
// adjustment ("+1", "-2"), then "#" or a JSON pointer.
func validRelativeJSONPointer(s string) bool {
	n := leadingInt(s)
	if n == 0 {
		return false
	}
	s = s[n:]
	if s != "" && (s[0] == '+' || s[0] == '-') {
		m := leadingInt(s[1:])
		if m == 0 || s[1] == '0' {
			return false
		}
		s = s[1+m:]
	}
	return s == "#" || validJSONPointer(s)
}

// leadingInt is the length of the non-negative integer (no leading zero)
// at the start of s, or 0.
func leadingInt(s string) int {
	if s == "" || s[0] < '0' || s[0] > '9' {
		return 0
	}
	if s[0] == '0' {
		return 1
	}
	n := 1
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}

// validURITemplate accepts an RFC 6570 URI template (level 4).
func validURITemplate(s string) bool {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 || !validExpression(s[i+1:i+end]) {
				return false
			}
			i += end + 1
			continue
		case r == '%':
			if i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2]) {
				return false
			}
			n = 3
		case r < 0x80:
			if r <= 0x20 || r == 0x7f || strings.ContainsRune("\"<>\\^`{|}", r) {
				return false
			}
		case !ucschar(r) && !iprivate(r):
			return false
		}
		i += n
	}
	return true
}

func validExpression(e string) bool {
	if e != "" && strings.IndexByte("+#./;?&", e[0]) >= 0 {
		e = e[1:]
	}
	for _, spec := range strings.Split(e, ",") {
		name := spec
		if strings.HasSuffix(spec, "*") {
			name = spec[:len(spec)-1]
		} else if c := strings.IndexByte(spec, ':'); c >= 0 {
			name = spec[:c]
			l := spec[c+1:]
			if l == "" || len(l) > 4 || l[0] == '0' || leadingInt(l) != len(l) {
				return false
			}
		}
		if !validVarname(name) {
			return false
		}
	}
	return true
}

// validVarname: varchars (letters, digits, "_" and percent-encodings)
// separated by single dots.
func validVarname(v string) bool {
	if v == "" || v[0] == '.' || v[len(v)-1] == '.' {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case isAlnum(rune(c)) || c == '_':
		case c == '.':
			if v[i+1] == '.' {
				return false
			}
		case c == '%':
			if i+2 >= len(v) || !isHex(v[i+1]) || !isHex(v[i+2]) {
				return false
			}
			i += 2
		default:
			return false
		}
	}
	return true
}
