// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package jsonschema

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
	"golang.org/x/text/cases"
	"golang.org/x/text/secure/bidirule"
	"golang.org/x/text/unicode/bidi"
	"golang.org/x/text/unicode/norm"
)

// Host names (RFC 1123 section 2.1) and internationalised host names
// (IDNA2008: RFC 5890, 5891, 5892 and 5893).
//
// A label of a host name is letters, digits and hyphens, 1 to 63 of them,
// neither starting nor ending with a hyphen; the name is at most 253
// characters and has no trailing dot. A label starting "xn--" must be an
// A-label: Punycode (RFC 3492) whose decoding re-encodes to it and is a
// valid U-label.
//
// An internationalised host name may also hold U-labels, and may separate
// labels with U+3002, U+FF0E or U+FF61. Each label is first mapped as a
// lookup would (UTS 46 section 4): characters that are default-ignorable
// (other than the two joiners) are dropped, and the rest is NFKC-normalised
// and case-folded. This approximates the IDNA mapping table rather than
// reading it. The mapped label is then checked by IDNA2008: NFC, the
// hyphen rules, no leading combining mark, every character PVALID or
// allowed by its CONTEXTJ or CONTEXTO rule, and the Bidi rule across the
// name. The A-label form of each label is at most 63 characters and the
// name at most 253.
//
// The derived property of RFC 5892 section 3 is computed from Go's Unicode
// tables (and those of golang.org/x/text), not read from the IANA table;
// the CONTEXTJ rules use golang.org/x/net/idna's joining types.

func validHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	var ulabels []string
	for _, l := range strings.Split(s, ".") {
		if !validLDH(l) {
			return false
		}
		if len(l) >= 4 && strings.EqualFold(l[:4], "xn--") {
			u, ok := decodeALabel(l)
			if !ok {
				return false
			}
			ulabels = append(ulabels, u)
		} else {
			ulabels = append(ulabels, l)
		}
	}
	return bidiOK(ulabels)
}

// validLDH: 1 to 63 letters, digits and hyphens, not at either end.
func validLDH(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		if !isAlnum(rune(l[i])) && l[i] != '-' {
			return false
		}
	}
	return true
}

// decodeALabel returns the U-label an A-label stands for, when it is a
// valid one.
func decodeALabel(l string) (string, bool) {
	a := strings.ToLower(l)
	u, err := punyDecode(a[4:])
	if err != nil || isASCII(u) {
		return "", false
	}
	if back, err := punyEncode(u); err != nil || back != a[4:] {
		return "", false
	}
	return u, validULabel(u)
}

func validIDNHostname(s string) bool {
	if s == "" {
		return false
	}
	s = strings.Map(func(r rune) rune {
		if r == '\U00003002' || r == '\U0000ff0e' || r == '\U0000ff61' {
			return '.'
		}
		return r
	}, s)
	var ulabels []string
	total := 0
	for _, l := range strings.Split(s, ".") {
		if l == "" {
			return false
		}
		m := idnMap(l)
		if m == "" || strings.ContainsRune(m, '.') {
			return false
		}
		var a string
		if isASCII(m) {
			if !validLDH(m) {
				return false
			}
			a = m
			if strings.HasPrefix(m, "xn--") {
				u, ok := decodeALabel(m)
				if !ok {
					return false
				}
				m = u
			}
		} else {
			if !validULabel(m) {
				return false
			}
			enc, err := punyEncode(m)
			if err != nil {
				return false
			}
			a = "xn--" + enc
			if len(a) > 63 {
				return false
			}
		}
		total += len(a) + 1
		ulabels = append(ulabels, m)
	}
	return total-1 <= 253 && bidiOK(ulabels)
}

// idnMap maps a label as a lookup would (see above).
func idnMap(l string) string {
	if isASCII(l) {
		return strings.ToLower(l)
	}
	// The UTS 46 mapping table (nontransitional) as golang.org/x/net/idna
	// carries it; its validation errors are ignored here, as the label is
	// checked by IDNA2008 below.
	m, _ := uts46Map.ToUnicode(l)
	return m
}

var uts46Map = idna.New(idna.MapForLookup(), idna.Transitional(false))

// validULabel checks a U-label by RFC 5891 section 5.4.
func validULabel(u string) bool {
	if u == "" || !norm.NFC.IsNormalString(u) {
		return false
	}
	rs := []rune(u)
	if len(rs) >= 4 && rs[2] == '-' && rs[3] == '-' || rs[0] == '-' || rs[len(rs)-1] == '-' {
		return false
	}
	if unicode.Is(unicode.M, rs[0]) {
		return false
	}
	joiners := false
	for i, r := range rs {
		switch idnaProperty(r) {
		case pvalid:
		case contextJ:
			joiners = true
		case contextO:
			if !contextORule(rs, i) {
				return false
			}
		default:
			return false
		}
	}
	if joiners {
		if _, err := joinerCheck.ToUnicode(u); err != nil {
			return false
		}
	}
	return true
}

// joinerCheck applies only the CONTEXTJ rules (RFC 5892 appendix A.1 and
// A.2) to a label.
var joinerCheck = idna.New(idna.CheckJoiners(true))

// bidiOK applies the Bidi rule (RFC 5893 section 2) to every label when
// one label holds a right-to-left or Arabic number character.
func bidiOK(labels []string) bool {
	rtl := false
	for _, l := range labels {
		if bidirule.DirectionString(l) != bidi.LeftToRight {
			rtl = true
			break
		}
	}
	if !rtl {
		return true
	}
	for _, l := range labels {
		if !bidirule.ValidString(l) {
			return false
		}
	}
	return true
}

type idnaProp int

const (
	disallowed idnaProp = iota
	pvalid
	contextJ
	contextO
)

// idnaProperty derives a code point's IDNA2008 property as RFC 5892
// section 3 orders the rules.
func idnaProperty(r rune) idnaProp {
	switch r { // exceptions, section 2.6
	case 0x00DF, 0x03C2, 0x06FD, 0x06FE, 0x0F0B, 0x3007:
		return pvalid
	case 0x00B7, 0x0375, 0x05F3, 0x05F4, 0x30FB:
		return contextO
	case 0x0640, 0x07FA, 0x302E, 0x302F, 0x3031, 0x3032, 0x3033, 0x3034, 0x3035, 0x303B:
		return disallowed
	}
	if r >= 0x0660 && r <= 0x0669 || r >= 0x06F0 && r <= 0x06F9 {
		return contextO
	}
	if !assigned(r) {
		return disallowed // unassigned
	}
	if r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' {
		return pvalid
	}
	if r == 0x200C || r == 0x200D {
		return contextJ
	}
	switch {
	case unstable(r),
		unicode.Is(unicode.White_Space, r), unicode.Is(unicode.Noncharacter_Code_Point, r), defaultIgnorable(r),
		r >= 0x20D0 && r <= 0x20FF, r >= 0x1D100 && r <= 0x1D24F, // ignorable blocks
		oldHangulJamo(r):
		return disallowed
	}
	if unicode.In(r, unicode.Ll, unicode.Lu, unicode.Lo, unicode.Nd, unicode.Lm, unicode.Mn, unicode.Mc) {
		return pvalid
	}
	return disallowed
}

func assigned(r rune) bool {
	return unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.C) ||
		unicode.Is(unicode.Noncharacter_Code_Point, r)
}

// unstable: NFKC case folding changes the code point.
func unstable(r rune) bool {
	s := string(r)
	return norm.NFKC.String(cases.Fold().String(norm.NFKC.String(s))) != s
}

// defaultIgnorable approximates Default_Ignorable_Code_Point from the
// properties Go's tables carry.
func defaultIgnorable(r rune) bool {
	if unicode.Is(unicode.White_Space, r) || r >= 0xFFF9 && r <= 0xFFFB || r >= 0x13430 && r <= 0x1343F ||
		unicode.Is(unicode.Prepended_Concatenation_Mark, r) {
		return false
	}
	return unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) || unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Variation_Selector, r)
}

func oldHangulJamo(r rune) bool {
	return r >= 0x1100 && r <= 0x11FF || r >= 0xA960 && r <= 0xA97C || r >= 0xD7B0 && r <= 0xD7C6 || r >= 0xD7CB && r <= 0xD7FB
}

// contextORule applies the CONTEXTO rules of RFC 5892 appendix A.3 to A.9
// to the code point at i.
func contextORule(rs []rune, i int) bool {
	r := rs[i]
	switch {
	case r == 0x00B7: // MIDDLE DOT: between two "l"
		return i > 0 && i < len(rs)-1 && rs[i-1] == 'l' && rs[i+1] == 'l'
	case r == 0x0375: // GREEK KERAIA: before a Greek character
		return i < len(rs)-1 && unicode.Is(unicode.Greek, rs[i+1])
	case r == 0x05F3 || r == 0x05F4: // HEBREW GERESH, GERSHAYIM: after a Hebrew character
		return i > 0 && unicode.Is(unicode.Hebrew, rs[i-1])
	case r == 0x30FB: // KATAKANA MIDDLE DOT: in a label with Japanese script
		for _, x := range rs {
			if x != 0x30FB && unicode.In(x, unicode.Hiragana, unicode.Katakana, unicode.Han) {
				return true
			}
		}
		return false
	case r >= 0x0660 && r <= 0x0669: // ARABIC-INDIC DIGITS: not with extended ones
		for _, x := range rs {
			if x >= 0x06F0 && x <= 0x06F9 {
				return false
			}
		}
		return true
	case r >= 0x06F0 && r <= 0x06F9: // EXTENDED ARABIC-INDIC DIGITS: not with plain ones
		for _, x := range rs {
			if x >= 0x0660 && x <= 0x0669 {
				return false
			}
		}
		return true
	}
	return false
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// Punycode, RFC 3492 section 6, with its parameters (section 5).
const (
	pBase        = 36
	pTMin        = 1
	pTMax        = 26
	pSkew        = 38
	pDamp        = 700
	pInitialBias = 72
	pInitialN    = 128
	pMaxInt      = 1<<31 - 1
)

var errPuny = errors.New("not Punycode")

func punyAdapt(delta, numPoints int, first bool) int {
	if first {
		delta /= pDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((pBase-pTMin)*pTMax)/2 {
		delta /= pBase - pTMin
		k += pBase
	}
	return k + (pBase-pTMin+1)*delta/(delta+pSkew)
}

func punyT(k, bias int) int {
	switch t := k - bias; {
	case t < pTMin:
		return pTMin
	case t > pTMax:
		return pTMax
	default:
		return t
	}
}

func punyDecode(in string) (string, error) {
	var out []rune
	pos := 0
	if b := strings.LastIndexByte(in, '-'); b >= 0 {
		for i := 0; i < b; i++ {
			if in[i] >= 0x80 {
				return "", errPuny
			}
			out = append(out, rune(in[i]))
		}
		pos = b + 1
	}
	n, i, bias := pInitialN, 0, pInitialBias
	for pos < len(in) {
		oldI, w := i, 1
		for k := pBase; ; k += pBase {
			if pos == len(in) {
				return "", errPuny
			}
			c := in[pos]
			pos++
			var d int
			switch {
			case c >= 'a' && c <= 'z':
				d = int(c - 'a')
			case c >= 'A' && c <= 'Z':
				d = int(c - 'A')
			case c >= '0' && c <= '9':
				d = int(c-'0') + 26
			default:
				return "", errPuny
			}
			if d > (pMaxInt-i)/w {
				return "", errPuny
			}
			i += d * w
			t := punyT(k, bias)
			if d < t {
				break
			}
			if w > pMaxInt/(pBase-t) {
				return "", errPuny
			}
			w *= pBase - t
		}
		x := len(out) + 1
		bias = punyAdapt(i-oldI, x, oldI == 0)
		if i/x > pMaxInt-n {
			return "", errPuny
		}
		n += i / x
		i %= x
		if n > unicode.MaxRune || n >= 0xD800 && n <= 0xDFFF {
			return "", errPuny
		}
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = rune(n)
		i++
	}
	return string(out), nil
}

func punyEncode(s string) (string, error) {
	rs := []rune(s)
	var out []byte
	for _, r := range rs {
		if r < 0x80 {
			out = append(out, byte(r))
		}
	}
	b := len(out)
	h := b
	if b > 0 {
		out = append(out, '-')
	}
	n, delta, bias := pInitialN, 0, pInitialBias
	for h < len(rs) {
		m := int(unicode.MaxRune) + 1
		for _, r := range rs {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if (m - n) > (pMaxInt-delta)/(h+1) {
			return "", errPuny
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range rs {
			if int(r) < n {
				delta++
				if delta > pMaxInt {
					return "", errPuny
				}
			}
			if int(r) == n {
				q := delta
				for k := pBase; ; k += pBase {
					t := punyT(k, bias)
					if q < t {
						break
					}
					out = append(out, punyDigit(t+(q-t)%(pBase-t)))
					q = (q - t) / (pBase - t)
				}
				out = append(out, punyDigit(q))
				bias = punyAdapt(delta, h+1, h == b)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out), nil
}

func punyDigit(d int) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('0' + d - 26)
}
